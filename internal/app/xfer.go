package app

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

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
	for _, h := range eff.JumpChain {
		if h.Mode == "shell-hop" {
			t.ShellHop = true
		}
	}
	return t, nil
}

// XferOpen opens an SFTP handle for sessionID. viaSessionID == "" uses the
// session's own connection; otherwise a new SSH connection to the target is
// dialed through viaSessionID's tunnel (auth prompts appear as usual, nothing
// is stored — ADR-0005). Returns the handle id.
func (a *App) XferOpen(sessionID, viaSessionID string) (string, error) {
	h := &xferHandle{}
	if viaSessionID == "" {
		client, ok := a.mgr.Client(sessionID)
		if !ok {
			return "", fmt.Errorf("app: session not connected")
		}
		raw := client.SSHClient()
		if raw == nil {
			return "", fmt.Errorf("app: this session runs through a shell-hop; choose a SOCKS session to copy through instead")
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
