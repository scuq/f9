package app

import (
	"golang.org/x/crypto/ssh"

	"context"
	"encoding/base64"
	"io"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/scuq/f9/internal/connmgr"
	"github.com/scuq/f9/internal/sshx"
	"github.com/scuq/f9/internal/store"
)

type fakeWriter struct {
	mu  sync.Mutex
	buf []byte
}

func (w *fakeWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	return len(p), nil
}
func (w *fakeWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.buf)
}

type fakeSession struct {
	stdin  *fakeWriter
	onData func([]byte)
	closed chan struct{}
}

func newFakeSession() *fakeSession {
	return &fakeSession{stdin: &fakeWriter{}, closed: make(chan struct{})}
}
func (s *fakeSession) Stdin() io.Writer      { return s.stdin }
func (s *fakeSession) OnData(f func([]byte)) { s.onData = f }
func (s *fakeSession) Resize(int, int) error { return nil }
func (s *fakeSession) Wait() error           { <-s.closed; return nil }
func (s *fakeSession) Close() error {
	select {
	case <-s.closed:
	default:
		close(s.closed)
	}
	return nil
}

type fakeTermClient struct{ sess *fakeSession }

func (c *fakeTermClient) NewSession(context.Context, string, int, int) (sshx.Session, error) {
	return c.sess, nil
}
func (c *fakeTermClient) ServerVersion() string   { return "SSH-2.0-fake" }
func (c *fakeTermClient) SocksActive() bool       { return false }
func (c *fakeTermClient) SSHClient() *ssh.Client  { return nil }
func (c *fakeTermClient) ConnInfo() sshx.ConnInfo { return sshx.ConnInfo{} }
func (c *fakeTermClient) Wait() error {
	var never chan struct{}
	<-never // block forever; client death isn't exercised in these tests
	return nil
}
func (c *fakeTermClient) Close() error { return nil }

func setupConnectedTerminal(t *testing.T) (*App, string, *fakeSession) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("F9_STORE", dir)
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutFolder(store.Folder{Name: "lab", ParentID: st.RootID()}); err != nil {
		t.Fatal(err)
	}
	lab, _ := st.FolderByName(st.RootID(), "lab")
	if err := st.Put(store.Session{Name: "x", FolderID: lab.ID, Host: "h", User: "u"}); err != nil {
		t.Fatal(err)
	}
	var id string
	for _, s := range st.Sessions() {
		id = s.ID
	}
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	fs := newFakeSession()
	dial := func(context.Context, string, int, string, sshx.Prompter, sshx.DialOpts) (sshx.Client, error) {
		return &fakeTermClient{sess: fs}, nil
	}
	a.mgr = connmgr.New(64, dial, a.emitConns)
	if err := a.ConnectSessions([]string{id}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := a.mgr.Client(id); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("session did not connect")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return a, id, fs
}

func TestTerminalLifecycle(t *testing.T) {
	a, id, fs := setupConnectedTerminal(t)

	var mu sync.Mutex
	var emitted []string
	var closed int
	a.onEmit = func(ev string, data interface{}) {
		mu.Lock()
		defer mu.Unlock()
		if ev == "f9:term:T1" {
			emitted = append(emitted, data.(string))
		}
		if ev == "f9:termclosed" {
			if mm, ok := data.(map[string]interface{}); ok && mm["termId"] == "T1" {
				closed++
			}
		}
	}
	if err := a.OpenTerminal("T1", id, 80, 24); err != nil {
		t.Fatal(err)
	}
	fs.onData([]byte("hello\r\n"))
	mu.Lock()
	got := append([]string(nil), emitted...)
	mu.Unlock()
	want := base64.StdEncoding.EncodeToString([]byte("hello\r\n"))
	if len(got) != 1 || got[0] != want {
		t.Fatalf("emitted = %v, want [%q]", got, want)
	}
	a.TermInput("T1", "show version\n")
	if fs.stdin.String() != "show version\n" {
		t.Fatalf("stdin = %q", fs.stdin.String())
	}
	a.CloseTerminal("T1")
	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		c := closed
		mu.Unlock()
		if c == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no f9:termclosed emitted")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestTerminalActivityMatch(t *testing.T) {
	a, id, fs := setupConnectedTerminal(t)
	if err := a.OpenTerminal("T1", id, 80, 24); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	kinds := map[string]int{}
	a.onEmit = func(ev string, data interface{}) {
		if ev == "f9:termactivity" {
			m := data.(map[string]string)
			mu.Lock()
			kinds[m["kind"]]++
			mu.Unlock()
		}
	}
	if err := a.SetTerminalWatch("T1", "ERROR"); err != nil {
		t.Fatal(err)
	}
	fs.onData([]byte("all good\r\n"))
	fs.onData([]byte("ERROR: disk full\r\n"))
	mu.Lock()
	m, o := kinds["match"], kinds["output"]
	mu.Unlock()
	if m < 1 {
		t.Fatalf("match not emitted: %v", kinds)
	}
	if o < 1 {
		t.Fatalf("output not emitted: %v", kinds)
	}
}

func TestTerminalActivityPrompt(t *testing.T) {
	a, id, fs := setupConnectedTerminal(t)
	if err := a.OpenTerminal("T1", id, 80, 24); err != nil {
		t.Fatal(err)
	}
	a.tmu.Lock()
	a.terms["T1"].promptRe = regexp.MustCompile(`\$ $`)
	a.tmu.Unlock()

	var mu sync.Mutex
	prompt := 0
	a.onEmit = func(ev string, data interface{}) {
		if ev == "f9:termactivity" && data.(map[string]string)["kind"] == "prompt" {
			mu.Lock()
			prompt++
			mu.Unlock()
		}
	}
	a.TermInput("T1", "sleep 1\n") // marks running
	fs.onData([]byte("scuq@lyrael:~$ "))
	mu.Lock()
	p := prompt
	mu.Unlock()
	if p < 1 {
		t.Fatal("prompt activity not emitted after command completion")
	}
}

func TestCloseLastTerminalDisconnects(t *testing.T) {
	a, id, _ := setupConnectedTerminal(t)
	if err := a.OpenTerminal("T1", id, 80, 24); err != nil {
		t.Fatal(err)
	}
	if err := a.OpenTerminal("T2", id, 80, 24); err != nil {
		t.Fatal(err)
	}
	a.CloseTerminal("T1")
	if _, ok := a.mgr.Client(id); !ok {
		t.Fatal("connection dropped while another terminal was still open")
	}
	a.CloseTerminal("T2")
	if _, ok := a.mgr.Client(id); ok {
		t.Fatal("connection still open after last terminal closed")
	}
}

func TestViewportScrollback(t *testing.T) {
	cases := map[int]int{0: viewportScrollbackDefault, -1: viewportScrollbackDefault, 500: 500, 5_000_000: viewportScrollbackMax}
	for in, want := range cases {
		if got := viewportScrollback(in); got != want {
			t.Errorf("viewportScrollback(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestTermLastOutput(t *testing.T) {
	a, id, fs := setupConnectedTerminal(t)
	if err := a.OpenTerminal("t1", id, 80, 24); err != nil {
		t.Fatal(err)
	}
	defer a.CloseTerminal("t1")
	fs.onData([]byte("banner\r\nrouter> "))
	a.TermInput("t1", "show x")
	fs.onData([]byte("show x")) // echoed while typing
	a.TermInput("t1", "\r")
	fs.onData([]byte("\r\nline1\r\nline2\r\nrouter> "))
	got, err := a.TermLastOutput("t1")
	if err != nil {
		t.Fatal(err)
	}
	if want := "line1\nline2\nrouter> "; got != want {
		t.Fatalf("last output = %q, want %q", got, want)
	}
	// With a prompt regex the trailing prompt line is dropped.
	a.tmu.Lock()
	a.terms["t1"].promptRe = regexp.MustCompile(`^router> $`)
	a.tmu.Unlock()
	got, err = a.TermLastOutput("t1")
	if err != nil {
		t.Fatal(err)
	}
	if want := "line1\nline2"; got != want {
		t.Fatalf("last output (prompt stripped) = %q, want %q", got, want)
	}
	// Bare Enter (fresh prompt) and Space+Enter (pager) do not move the mark.
	a.TermInput("t1", "\r")
	fs.onData([]byte("\r\nrouter> "))
	a.TermInput("t1", " ")
	a.TermInput("t1", "\r")
	fs.onData([]byte("\r\nrouter> "))
	got, _ = a.TermLastOutput("t1")
	if want := "line1\nline2\nrouter> \nrouter> "; got != want {
		t.Fatalf("after bare enters = %q, want %q", got, want)
	}
	// A real second command moves the mark; the earlier output is excluded.
	// Keystrokes arrive one per input call, like a typed command.
	for _, ch := range []string{"s", "h", "o", "w", " ", "y"} {
		fs.onData([]byte(ch))
		a.TermInput("t1", ch)
	}
	a.TermInput("t1", "\n")
	fs.onData([]byte("\r\nonly\r\n"))
	got, _ = a.TermLastOutput("t1")
	if got != "only" {
		t.Fatalf("after second command = %q, want %q", got, "only")
	}
	// A pasted multi-line command marks at its last line.
	fs.onData([]byte("router> "))
	a.TermInput("t1", "conf t\nhostname r\n")
	fs.onData([]byte("\r\nrouter(config)# \r\nrouter(config)# "))
	got, _ = a.TermLastOutput("t1")
	if want := "router(config)# \nrouter(config)# "; got != want {
		t.Fatalf("after paste = %q, want %q", got, want)
	}
	if _, err := a.TermLastOutput("nope"); err == nil {
		t.Fatal("expected error for unknown terminal")
	}
}
