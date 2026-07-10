package app

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scuq/f9/internal/connmgr"
	"github.com/scuq/f9/internal/sshx"
	"github.com/scuq/f9/internal/store"
)

// mockRequester counts prompt requests and returns a scripted reply.
type mockRequester struct {
	mu    sync.Mutex
	calls int
	reply PromptReply
}

func (m *mockRequester) requestPrompt(req PromptRequest) (PromptReply, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	return m.reply, nil
}

func TestPromptBridgePasswordUseForAll(t *testing.T) {
	r := &mockRequester{reply: PromptReply{Value: "pw", UseForAll: true}}
	b := newPromptBridge(r)
	for i := 0; i < 3; i++ {
		v, err := b.Password("admin", "h")
		if err != nil || v != "pw" {
			t.Fatalf("Password #%d = %q, %v", i, v, err)
		}
	}
	if r.calls != 1 {
		t.Fatalf("requests = %d, want 1 (use-for-all)", r.calls)
	}
}

func TestPromptBridgePasswordPerSession(t *testing.T) {
	r := &mockRequester{reply: PromptReply{Value: "pw", UseForAll: false}}
	b := newPromptBridge(r)
	for i := 0; i < 3; i++ {
		if _, err := b.Password("admin", "h"); err != nil {
			t.Fatal(err)
		}
	}
	if r.calls != 3 {
		t.Fatalf("requests = %d, want 3 (per session)", r.calls)
	}
}

func TestPromptBridgePassphraseSharedByPath(t *testing.T) {
	r := &mockRequester{reply: PromptReply{Value: "s3cret"}}
	b := newPromptBridge(r)
	b.Passphrase("/home/scuq/.ssh/id_ed25519")
	b.Passphrase("/home/scuq/.ssh/id_ed25519")
	b.Passphrase("/home/scuq/.ssh/id_rsa")
	if r.calls != 2 {
		t.Fatalf("requests = %d, want 2 (per path)", r.calls)
	}
}

func TestPromptBridgeHostKeyAlwaysPrompts(t *testing.T) {
	r := &mockRequester{reply: PromptReply{Accept: true}}
	b := newPromptBridge(r)
	b.ConfirmHostKey("h1", "fp1")
	b.ConfirmHostKey("h2", "fp2")
	if r.calls != 2 {
		t.Fatalf("requests = %d, want 2 (per host)", r.calls)
	}
}

// fakeGUIClient is a minimal sshx.Client for the integration test.
type fakeGUIClient struct{}

func (fakeGUIClient) NewSession(_ context.Context, _ string, _, _ int) (sshx.Session, error) {
	return nil, nil
}
func (fakeGUIClient) ServerVersion() string { return "SSH-2.0-fake" }
func (fakeGUIClient) SocksActive() bool     { return false }
func (fakeGUIClient) Wait() error {
	var never chan struct{}
	<-never // block forever; this fake connection never dies on its own
	return nil
}
func (fakeGUIClient) Close() error { return nil }

// TestConnectSessionsSharedPassword drives the full app path: a fake dial that
// invokes the batch prompter, an onEmit hook that auto-answers with
// use-for-all, and asserts both sessions connect with a single prompt.
func TestConnectSessionsSharedPassword(t *testing.T) {
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
	var ids []string
	for _, n := range []string{"a", "b"} {
		if err := st.Put(store.Session{Name: n, FolderID: lab.ID, Host: "h", User: "admin"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range st.Sessions() {
		ids = append(ids, s.ID)
	}

	a, err := New()
	if err != nil {
		t.Fatal(err)
	}

	var promptCount atomic.Int32
	a.onEmit = func(ev string, data interface{}) {
		if ev != "f9:prompt" {
			return
		}
		promptCount.Add(1)
		req := data.(PromptRequest)
		a.ResolvePrompt(req.ID, PromptReply{Value: "pw", UseForAll: true})
	}

	// fake dial that actually exercises the prompter (password auth)
	dial := func(_ context.Context, _ string, _ int, _ string, p sshx.Prompter, _ sshx.DialOpts) (sshx.Client, error) {
		if _, err := p.Password("admin", "h"); err != nil {
			return nil, err
		}
		return fakeGUIClient{}, nil
	}
	a.mgr = connmgr.New(64, dial, a.emitConns)

	if err := a.ConnectSessions(ids); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		conns := a.ActiveConnections()
		connected := 0
		for _, c := range conns {
			if c.State == connmgr.StateConnected {
				connected++
			}
		}
		if connected == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("sessions did not both connect: %+v", conns)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := promptCount.Load(); got != 1 {
		t.Fatalf("prompt count = %d, want 1 (shared password)", got)
	}
}
