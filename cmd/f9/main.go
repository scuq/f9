// f9 — cross-platform SSH client. This binary is the phase-00 CLI smoke
// harness and stays forever as the headless test tool. See docs/phase-plan.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"golang.org/x/term"

	"github.com/scuq/f9/internal/osdetect"
	"github.com/scuq/f9/internal/scrollback"
	"github.com/scuq/f9/internal/sshx"
	"github.com/scuq/f9/internal/store"
)

const version = "0.0.4-phase00e"

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
		if err := cmdConnect(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "f9 connect:", err)
			os.Exit(1)
		}
	case "grep":
		if err := cmdGrep(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "f9 grep:", err)
			os.Exit(1)
		}
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

func openStore() (*store.YAMLStore, error) {
	root, err := storeRoot()
	if err != nil {
		return nil, err
	}
	return store.Open(root)
}

func cmdList() error {
	st, err := openStore()
	if err != nil {
		return err
	}
	sessions := st.Sessions()
	if len(sessions) == 0 {
		root, _ := storeRoot()
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

func cmdConnect(args []string) error {
	fs := flag.NewFlagSet("connect", flag.ContinueOnError)
	record := fs.Bool("record", false, "record session output for f9 grep")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: f9 connect [-record] <session | folder/name>")
	}

	st, err := openStore()
	if err != nil {
		return err
	}
	sess, err := findSession(st, fs.Arg(0))
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

	var rec *recorder
	if *record {
		rdir, err := recordingsDir()
		if err != nil {
			return err
		}
		path := filepath.Join(rdir, sess.ID+".zst")
		if rec, err = newRecorder(path); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "recording to %s\n", path)
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
		if rec != nil {
			rec.Close()
		}
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
		if rec != nil {
			rec.Close()
		}
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
	// to the first handler only, and both the detector and the recorder must
	// see those first bytes.
	sshSess.OnData(func(b []byte) {
		_, _ = os.Stdout.Write(b)
		det.ObserveOutput(b)
		if rec != nil {
			_ = rec.Write(b)
		}
	})
	go func() { _, _ = io.Copy(sshSess.Stdin(), os.Stdin) }()
	go watchResize(fd, sshSess)

	_ = sshSess.Wait()
	if restore != nil {
		restore()
	}
	fmt.Fprintln(os.Stderr, "\nconnection closed")

	if rec != nil {
		if err := rec.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "recording close:", err)
		} else {
			fmt.Fprintf(os.Stderr, "recorded %d bytes (uncompressed)\n", rec.Bytes())
		}
	}

	// Persist the OS guess passively gathered during the session (00d).
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

// cmdGrep replays a recorded session through the scrollback buffer (the real
// 00b hot path on real data) and streams matches grep-style.
func cmdGrep(args []string) error {
	fs := flag.NewFlagSet("grep", flag.ContinueOnError)
	invert := fs.Bool("v", false, "invert match")
	icase := fs.Bool("i", false, "ignore case")
	after := fs.Int("A", 0, "lines of after-context")
	before := fs.Int("B", 0, "lines of before-context")
	maxM := fs.Int("m", 0, "stop after N matches (0 = unlimited)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("usage: f9 grep [-v -i -A n -B n -m n] <session | folder/name> <pattern>")
	}

	st, err := openStore()
	if err != nil {
		return err
	}
	sess, err := findSession(st, fs.Arg(0))
	if err != nil {
		return err
	}
	re, err := regexp.Compile(fs.Arg(1))
	if err != nil {
		return fmt.Errorf("pattern: %w", err)
	}

	rdir, err := recordingsDir()
	if err != nil {
		return err
	}
	path := filepath.Join(rdir, sess.ID+".zst")
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("no recording for %q — run: f9 connect -record %s", sess.Name, sess.Name)
	}
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := zstd.NewReader(f)
	if err != nil {
		return fmt.Errorf("open recording: %w", err)
	}
	defer zr.Close()

	buf := scrollback.New(scrollback.Config{})
	defer buf.Close()
	chunk := make([]byte, 64<<10)
	for {
		n, rerr := zr.Read(chunk)
		if n > 0 {
			buf.Append(chunk[:n])
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("read recording: %w", rerr)
		}
	}

	it, err := buf.Grep(re, scrollback.GrepOpts{
		Invert:     *invert,
		IgnoreCase: *icase,
		After:      *after,
		Before:     *before,
		MaxMatches: *maxM,
	})
	if err != nil {
		return err
	}
	defer it.Close()

	withContext := *after > 0 || *before > 0
	last := -1 // highest 0-based line number printed
	printedAny := false
	count := 0
	for {
		m, ok := it.Next()
		if !ok {
			break
		}
		count++
		start := m.LineNo - len(m.Before)
		if withContext && printedAny && start > last+1 {
			fmt.Println("--")
		}
		for idx, ln := range m.Before {
			no := start + idx
			if no <= last {
				continue
			}
			fmt.Printf("%d-%s\n", no+1, ln)
			last = no
		}
		if m.LineNo > last {
			fmt.Printf("%d:%s\n", m.LineNo+1, m.Line)
			last = m.LineNo
		}
		for idx, ln := range m.After {
			no := m.LineNo + 1 + idx
			if no <= last {
				continue
			}
			fmt.Printf("%d-%s\n", no+1, ln)
			last = no
		}
		printedAny = true
	}
	if err := it.Close(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "%d matches\n", count)
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: f9 <command>

commands:
  list                                          list sessions (F9_STORE overrides path)
  connect [-record] <session | folder/name>     interactive attach (exit via remote logout)
  grep [-v -i -A n -B n -m n] <session> <re>    grep a recorded session (see connect -record)
  version                                       print version`)
}
