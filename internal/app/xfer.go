package app

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/scuq/f9/internal/sshx"
	"github.com/scuq/f9/internal/store"
	"github.com/scuq/f9/internal/xfer"
)

// File upload (SFTP) bindings. An "xfer" is a browsing/upload handle the
// upload dialog holds open; it either rides the session's own SSH client or
// a fresh SSH connection to the target tunnelled through another session's
// connection (the "via SOCKS session" route, for targets only reachable
// from there or behind a shell-hop).

type xferHandle struct {
	conn   *xfer.Conn
	owned  sshx.Client // non-nil when this xfer dialed its own connection
	cancel context.CancelFunc
}

// XferTarget describes what the dialog is about to talk to.
type XferTarget struct {
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	User      string `json:"user"`
	ShellHop  bool   `json:"shellHop"` // session's own connection cannot carry SFTP
	HopLabel  string `json:"hopLabel"` // "user@host" of the shell-hop, for the route picker
}

type XferListing struct {
	Dir     string       `json:"dir"`
	Entries []xfer.Entry `json:"entries"`
}

type XferProgress struct {
	ID       string `json:"id"`
	File     string `json:"file"`
	Done     int64  `json:"done"`
	Total    int64  `json:"total"`
	Finished bool   `json:"finished"`
	Error    string `json:"error"`
}

// XferTargetFor resolves the upload target for a terminal tab.
func (a *App) XferTargetFor(termID string) (XferTarget, error) {
	a.tmu.Lock()
	sessionID, ok := a.termSess[termID]
	a.tmu.Unlock()
	if !ok {
		return XferTarget{}, fmt.Errorf("app: terminal not open")
	}
	s, eff, err := a.st.Resolve(sessionID)
	if err != nil {
		return XferTarget{}, err
	}
	t := XferTarget{SessionID: s.ID, Name: s.Name, Host: s.Host, Port: s.Port, User: s.User}
	if rt, err := a.targetFor(sessionID); err == nil {
		t.User = rt.User // alt-user labels (@name) resolved, as the dial sees them
	}
	for _, h := range eff.JumpChain {
		if h.Mode == "shell-hop" {
			t.ShellHop = true
			t.HopLabel = h.Host
			if h.User != "" {
				t.HopLabel = h.User + "@" + h.Host
			}
		}
	}
	return t, nil
}

// XferViaHop is the viaSessionID value selecting the "through the jump host"
// route: the hop's own ssh binary opens the target's sftp subsystem, so the
// hop's keys/agent authenticate — exactly as the interactive shell-hop does.
const XferViaHop = "@hop"

// hopClientOf returns the jump host's SSH client for a shell-hop session.
func hopClientOf(c sshx.Client) (*ssh.Client, bool) {
	h, ok := c.(interface{ HopClient() *ssh.Client })
	if !ok {
		return nil, false
	}
	return h.HopClient(), true
}

// hopSession is the carrier for the hop route: one exec session on the jump
// host running ssh -s ... sftp; closing it ends that process.
type hopSession struct {
	sess   *ssh.Session
	stderr *bytes.Buffer
}

func (h *hopSession) Close() error { return h.sess.Close() }

func (a *App) openViaHop(sessionID string) (*xfer.Conn, error) {
	client, ok := a.mgr.Client(sessionID)
	if !ok {
		return nil, fmt.Errorf("app: session not connected")
	}
	hop, ok := hopClientOf(client)
	if !ok || hop == nil {
		return nil, fmt.Errorf("app: this session has no shell-hop; use its own connection")
	}
	t, err := a.targetFor(sessionID)
	if err != nil {
		return nil, err
	}
	cmd, err := sshx.SFTPViaHopCommand(t.Host, t.Port, t.User)
	if err != nil {
		return nil, err
	}
	// Attempt 1: plain exec. Attempt 2: through a login shell, because a
	// non-interactive exec on the hop does not source the profile that the
	// interactive login did — typically where SSH_AUTH_SOCK / agent keys come
	// from. (cmd passed the safeArg allowlist, so single-quoting is safe.)
	attempts := []string{cmd, "sh -lc '" + cmd + "'"}
	var errs []string
	for _, c := range attempts {
		conn, why := a.startHopSFTP(hop, c)
		if why == nil {
			return conn, nil
		}
		errs = append(errs, why.Error())
	}
	return nil, fmt.Errorf("app: sftp through the jump host failed\n\u2022 %s", strings.Join(errs, "\n\u2022 "))
}

// startHopSFTP runs one sftp-carrier command on the hop and attaches the
// SFTP client. On failure it waits (bounded) for the process so its stderr —
// the hop's ssh explaining why — is complete, and returns it in the error.
func (a *App) startHopSFTP(hop *ssh.Client, cmd string) (*xfer.Conn, error) {
	sess, err := hop.NewSession()
	if err != nil {
		return nil, fmt.Errorf("jump host session: %w", err)
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		return nil, err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		return nil, err
	}
	hs := &hopSession{sess: sess, stderr: &bytes.Buffer{}}
	sess.Stderr = &limitedBuffer{b: hs.stderr, max: 16384}
	if err := sess.Start(cmd); err != nil {
		sess.Close()
		return nil, fmt.Errorf("start %q: %w", cmd, err)
	}
	conn, err := xfer.OpenPipe(stdout, stdin, hs)
	if err == nil {
		return conn, nil
	}
	// let the hop's ssh finish writing its complaint
	done := make(chan struct{})
	go func() { _ = sess.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
	sess.Close()
	msg := strings.TrimSpace(hs.stderr.String())
	if msg == "" {
		return nil, fmt.Errorf("%q: %v (the hop's ssh printed nothing)", cmd, err)
	}
	// keep the last 25 lines of the -v narration: the end is where it died
	lines := strings.Split(msg, "\n")
	if len(lines) > 25 {
		lines = lines[len(lines)-25:]
	}
	return nil, fmt.Errorf("%q: %v\n  jump host ssh said:\n  %s", cmd, err, strings.Join(lines, "\n  "))
}

// limitedBuffer keeps the tail of stderr bounded.
type limitedBuffer struct {
	b   *bytes.Buffer
	max int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	l.b.Write(p)
	if l.b.Len() > l.max {
		tail := l.b.Bytes()[l.b.Len()-l.max:]
		nb := bytes.NewBuffer(append([]byte(nil), tail...))
		*l.b = *nb
	}
	return len(p), nil
}

// XferOpen opens an SFTP handle for sessionID. viaSessionID == "" uses the
// session's own connection; otherwise a new SSH connection to the target is
// dialed through viaSessionID's tunnel (auth prompts appear as usual, nothing
// is stored — ADR-0005). Returns the handle id.
func (a *App) XferOpen(sessionID, viaSessionID string) (string, error) {
	h := &xferHandle{}
	if viaSessionID == XferViaHop {
		c, err := a.openViaHop(sessionID)
		if err != nil {
			return "", err
		}
		h.conn = c
	} else if viaSessionID == "" {
		client, ok := a.mgr.Client(sessionID)
		if !ok {
			return "", fmt.Errorf("app: session not connected")
		}
		raw := client.SSHClient()
		if raw == nil {
			return "", fmt.Errorf("app: this session runs through a shell-hop; choose the jump-host route (or a SOCKS session) instead")
		}
		c, err := xfer.Open(raw)
		if err != nil {
			return "", err
		}
		h.conn = c
	} else {
		via, ok := a.mgr.Client(viaSessionID)
		if !ok {
			return "", fmt.Errorf("app: SOCKS session not connected")
		}
		viaRaw := via.SSHClient()
		if viaRaw == nil {
			return "", fmt.Errorf("app: the SOCKS session is a shell-hop connection and cannot tunnel")
		}
		t, err := a.targetFor(sessionID)
		if err != nil {
			return "", err
		}
		opts := dialOptsFor(t)
		opts.JumpChain = nil // the tunnel replaces the session's own chain
		opts.SocksPort = 0
		opts.Via = viaRaw
		opts.Timeout = 20 * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		client, err := sshx.Dial(ctx, t.Host, t.Port, t.User, newPromptBridge(a), opts)
		cancel()
		if err != nil {
			return "", fmt.Errorf("app: dial %s via %s: %w", t.Host, viaSessionID, err)
		}
		c, err := xfer.Open(client.SSHClient())
		if err != nil {
			client.Close()
			return "", err
		}
		h.conn = c
		h.owned = client
	}
	id := store.NewULID()
	a.xferMu.Lock()
	if a.xfers == nil {
		a.xfers = map[string]*xferHandle{}
	}
	a.xfers[id] = h
	a.xferMu.Unlock()
	return id, nil
}

func (a *App) xfer(id string) (*xferHandle, error) {
	a.xferMu.Lock()
	h, ok := a.xfers[id]
	a.xferMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("app: transfer handle closed")
	}
	return h, nil
}

// XferList lists dir ("" or "~" = remote home) and returns the cleaned path.
func (a *App) XferList(id, dir string) (XferListing, error) {
	h, err := a.xfer(id)
	if err != nil {
		return XferListing{}, err
	}
	d, err := h.conn.Clean(dir)
	if err != nil {
		return XferListing{}, err
	}
	ents, err := h.conn.List(d)
	if err != nil {
		return XferListing{}, err
	}
	return XferListing{Dir: d, Entries: ents}, nil
}

func (a *App) XferMkdir(id, dir string) error {
	h, err := a.xfer(id)
	if err != nil {
		return err
	}
	d, err := h.conn.Clean(dir)
	if err != nil {
		return err
	}
	return h.conn.Mkdir(d)
}

// XferUpload uploads local files sequentially into remoteDir, reporting
// f9:xferprogress per file. Returns immediately; XferClose cancels.
func (a *App) XferUpload(id string, locals []string, remoteDir string) error {
	h, err := a.xfer(id)
	if err != nil {
		return err
	}
	d, err := h.conn.Clean(remoteDir)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.xferMu.Lock()
	if h.cancel != nil {
		a.xferMu.Unlock()
		cancel()
		return fmt.Errorf("app: an upload is already running on this handle")
	}
	h.cancel = cancel
	a.xferMu.Unlock()
	go func() {
		defer func() {
			a.xferMu.Lock()
			h.cancel = nil
			a.xferMu.Unlock()
			cancel()
		}()
		for _, l := range locals {
			name := filepath.Base(l)
			emit := func(done, total int64) {
				a.emitEvent("f9:xferprogress", XferProgress{ID: id, File: name, Done: done, Total: total})
			}
			err := h.conn.Upload(ctx, l, d, emit)
			p := XferProgress{ID: id, File: name, Finished: true}
			if err != nil {
				p.Error = err.Error()
			}
			a.emitEvent("f9:xferprogress", p)
			if err != nil && ctx.Err() != nil {
				return
			}
		}
	}()
	return nil
}

// XferClose cancels any running upload and releases the handle (and the
// private connection, if the handle dialed one).
func (a *App) XferClose(id string) {
	a.xferMu.Lock()
	h, ok := a.xfers[id]
	delete(a.xfers, id)
	a.xferMu.Unlock()
	if !ok {
		return
	}
	if h.cancel != nil {
		h.cancel()
	}
	_ = h.conn.Close()
	if h.owned != nil {
		_ = h.owned.Close()
	}
}
