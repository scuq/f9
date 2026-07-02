// f9 — cross-platform SSH client. This binary is the phase-00 CLI smoke
// harness and stays forever as the headless test tool. See docs/phase-plan.md.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/scuq/f9/internal/osdetect"
	"github.com/scuq/f9/internal/sshx"
	"github.com/scuq/f9/internal/store"
)

const version = "0.0.3-phase00d"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println("f9", version)
	case "list":
		if err := cmdList(); err != nil {
			fmt.Fprintln(os.Stderr, "f9 list:", err)
			os.Exit(1)
		}
	case "connect":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: f9 connect <session-name | folder/name>")
			os.Exit(2)
		}
		if err := cmdConnect(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "f9 connect:", err)
			os.Exit(1)
		}
	case "grep":
		fmt.Fprintln(os.Stderr, "f9 grep: recorded-buffer grep arrives with phase 00e")
		os.Exit(1)
	default:
		usage()
		os.Exit(2)
	}
}

func storeRoot() (string, error) {
	if p := os.Getenv("F9_STORE"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(home, ".config", "f9", "sessions"), nil
}

func cmdList() error {
	root, err := storeRoot()
	if err != nil {
		return err
	}
	st, err := store.Open(root)
	if err != nil {
		return err
	}
	sessions := st.Sessions()
	if len(sessions) == 0 {
		fmt.Printf("no sessions in %s (set F9_STORE to use another store)\n", root)
		return nil
	}
	for _, s := range sessions {
		target := s.Host
		if s.User != "" {
			target = s.User + "@" + target
		}
		if s.Port != 0 && s.Port != 22 {
			target = fmt.Sprintf("%s:%d", target, s.Port)
		}
		osTag := ""
		if m, err := st.Meta(s.ID); err == nil && m.DetectedOS != "" {
			osTag = m.DetectedOS
			if m.OSPinned {
				osTag += " (pinned)"
			}
		}
		fmt.Printf("%-44s %-32s %-4s %s\n", st.FolderPath(s.FolderID)+"/"+s.Name, target, s.Proto, osTag)
	}
	return nil
}

func findSession(st *store.YAMLStore, arg string) (store.Session, error) {
	var hits []store.Session
	larg := strings.ToLower(arg)
	for _, s := range st.Sessions() {
		full := strings.ToLower(st.FolderPath(s.FolderID) + "/" + s.Name)
		if strings.EqualFold(s.Name, arg) || full == larg || strings.HasSuffix(full, "/"+larg) {
			hits = append(hits, s)
		}
	}
	switch len(hits) {
	case 0:
		return store.Session{}, fmt.Errorf("no session matches %q", arg)
	case 1:
		return hits[0], nil
	default:
		var b strings.Builder
		for _, h := range hits {
			fmt.Fprintf(&b, "\n  %s/%s", st.FolderPath(h.FolderID), h.Name)
		}
		return store.Session{}, fmt.Errorf("%q is ambiguous, matches:%s", arg, b.String())
	}
}

func cmdConnect(arg string) error {
	root, err := storeRoot()
	if err != nil {
		return err
	}
	st, err := store.Open(root)
	if err != nil {
		return err
	}
	sess, err := findSession(st, arg)
	if err != nil {
		return err
	}
	_, eff, err := st.Resolve(sess.ID)
	if err != nil {
		return err
	}

	hops := make([]sshx.Hop, 0, len(eff.JumpChain))
	targetUser := sess.User
	for i, j := range eff.JumpChain {
		hops = append(hops, sshx.Hop{Host: j.Host, Port: j.Port, User: j.User, Mode: j.Mode})
		if i == len(eff.JumpChain)-1 && j.Mode == "shell-hop" && j.UserOverride != "" {
			targetUser = j.UserOverride
		}
	}

	termType := "xterm-256color"
	if eff.TermType != nil {
		termType = *eff.TermType
	}
	keepalive := 30 * time.Second
	if eff.KeepaliveInterval != nil {
		keepalive = *eff.KeepaliveInterval
	}

	det := osdetect.New()
	p := newCLIPrompter()
	fmt.Fprintf(os.Stderr, "connecting to %s (%s)...\n", sess.Name, sess.Host)
	client, err := sshx.Dial(context.Background(), sess.Host, sess.Port, targetUser, p, sshx.DialOpts{
		Timeout:           10 * time.Second,
		KeepaliveInterval: keepalive,
		JumpChain:         hops,
		OnBanner: func(b string) {
			det.ObserveOutput([]byte(b))
			fmt.Fprint(os.Stderr, b)
		},
	})
	if err != nil {
		return err
	}
	defer client.Close()
	det.ObserveServerVersion(client.ServerVersion())

	if m, err := st.Meta(sess.ID); err == nil {
		m.LastConnect = time.Now().UTC()
		_ = st.PutMeta(m)
	}

	fd := int(os.Stdin.Fd())
	cols, rows := 80, 24
	if term.IsTerminal(fd) {
		if c, r, err := term.GetSize(fd); err == nil {
			cols, rows = c, r
		}
	}
	sshSess, err := client.NewSession(context.Background(), termType, cols, rows)
	if err != nil {
		return err
	}
	defer sshSess.Close()

	var restore func()
	if term.IsTerminal(fd) {
		old, err := term.MakeRaw(fd)
		if err == nil {
			restore = func() { _ = term.Restore(fd, old) }
			defer restore()
		}
	}

	// One combined handler: the pre-registration pending buffer is replayed
	// to the first handler only, and the detector must see those first bytes
	// (banner + motd are prime evidence).
	sshSess.OnData(func(b []byte) {
		_, _ = os.Stdout.Write(b)
		det.ObserveOutput(b)
	})
	go func() { _, _ = io.Copy(sshSess.Stdin(), os.Stdin) }()
	go watchResize(fd, sshSess)

	_ = sshSess.Wait()
	if restore != nil {
		restore()
	}
	fmt.Fprintln(os.Stderr, "\nconnection closed")

	// Persist the OS guess passively gathered during the session (00d).
	// Shell-hop caveat: evidence past the hop belongs to the target, but the
	// server version is the hop's — the version rules just carry less weight.
	if g := det.Guess(); g.Family != osdetect.FamilyUnknown && g.Confidence >= osdetect.DefaultThreshold {
		if m, err := st.Meta(sess.ID); err == nil && !m.OSPinned {
			m.DetectedOS = string(g.Family)
			m.OSConfidence = g.Confidence
			_ = st.PutMeta(m)
		}
		fmt.Fprintf(os.Stderr, "detected OS: %s (confidence %.2f)\n", g.Family, g.Confidence)
	}
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: f9 <command>

commands:
  list                              list sessions (F9_STORE overrides path)
  connect <session | folder/name>   interactive attach (exit via remote logout)
  grep <session> <re>               grep a recorded scrollback buffer (00e)
  version                           print version`)
}
