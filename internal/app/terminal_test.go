package app

import (
	"context"
	"encoding/base64"
	"io"
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
func (c *fakeTermClient) ServerVersion() string { return "SSH-2.0-fake" }
func (c *fakeTermClient) Close() error          { return nil }

func TestTerminalLifecycle(t *testing.T) {
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

	var mu sync.Mutex
	var emitted []string
	a.onEmit = func(ev string, data interface{}) {
		if ev == "f9:term:"+id {
			mu.Lock()
			emitted = append(emitted, data.(string))
			mu.Unlock()
		}
	}

	if err := a.OpenTerminal(id, 80, 24); err != nil {
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

	a.TermInput(id, "show version\n")
	if fs.stdin.String() != "show version\n" {
		t.Fatalf("stdin = %q", fs.stdin.String())
	}

	if err := a.OpenTerminal(id, 80, 24); err != nil { // idempotent
		t.Fatal(err)
	}

	a.CloseTerminal(id)
	a.tmu.Lock()
	_, stillOpen := a.terms[id]
	a.tmu.Unlock()
	if stillOpen {
		t.Fatal("terminal not removed on close")
	}
}
