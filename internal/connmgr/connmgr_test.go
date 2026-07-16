package connmgr

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scuq/f9/internal/sshx"
)

type fakeClient struct {
	closed atomic.Bool
	mu     sync.Mutex
	done   chan struct{}
}

func (f *fakeClient) NewSession(_ context.Context, _ string, _, _ int) (sshx.Session, error) {
	return nil, nil
}
func (f *fakeClient) ServerVersion() string   { return "SSH-2.0-fake" }
func (f *fakeClient) SocksActive() bool       { return false }
func (f *fakeClient) ConnInfo() sshx.ConnInfo { return sshx.ConnInfo{} }
func (f *fakeClient) Wait() error {
	f.mu.Lock()
	if f.done == nil {
		f.done = make(chan struct{})
	}
	ch := f.done
	f.mu.Unlock()
	<-ch
	return nil
}

// die ends Wait() without marking the client closed — simulates a transport
// death so tests can assert that connmgr itself closes the wrapper.
func (f *fakeClient) die() {
	f.mu.Lock()
	if f.done == nil {
		f.done = make(chan struct{})
	}
	select {
	case <-f.done:
	default:
		close(f.done)
	}
	f.mu.Unlock()
}

func (f *fakeClient) Close() error {
	f.closed.Store(true)
	f.mu.Lock()
	if f.done == nil {
		f.done = make(chan struct{})
	}
	select {
	case <-f.done:
	default:
		close(f.done)
	}
	f.mu.Unlock()
	return nil
}

func TestWatchRemovesOnClientDeath(t *testing.T) {
	fc := &fakeClient{}
	dial := func(_ context.Context, _ string, _ int, _ string, _ sshx.Prompter, _ sshx.DialOpts) (sshx.Client, error) {
		return fc, nil
	}
	m := New(0, dial, func() {})
	<-m.ConnectBatch(context.Background(), []Target{{SessionID: "s1", Host: "h"}}, nil)
	if len(m.Active()) != 1 {
		t.Fatalf("want 1 active, got %d", len(m.Active()))
	}
	fc.Close() // simulate host death -> Wait() returns
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(m.Active()) != 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if n := len(m.Active()); n != 0 {
		t.Fatalf("dead connection should be removed, got %d", n)
	}
}

func targets(n int) []Target {
	out := make([]Target, n)
	for i := 0; i < n; i++ {
		out[i] = Target{SessionID: string(rune('a' + i)), Name: "s", Host: "h"}
	}
	return out
}

func waitDone(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("batch did not settle within deadline")
	}
}

func TestConcurrencyBound(t *testing.T) {
	var cur, max int32
	var mu sync.Mutex
	dial := func(_ context.Context, _ string, _ int, _ string, _ sshx.Prompter, _ sshx.DialOpts) (sshx.Client, error) {
		c := atomic.AddInt32(&cur, 1)
		mu.Lock()
		if c > max {
			max = c
		}
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&cur, -1)
		return &fakeClient{}, nil
	}
	m := New(4, dial, nil)
	waitDone(t, m.ConnectBatch(context.Background(), targets(20), nil))
	if max > 4 {
		t.Fatalf("max concurrent dials = %d, want <= 4", max)
	}
	if max < 2 {
		t.Fatalf("suspiciously low concurrency: %d", max)
	}
}

func TestStateAndDisconnect(t *testing.T) {
	fc := &fakeClient{}
	dial := func(_ context.Context, _ string, _ int, _ string, _ sshx.Prompter, _ sshx.DialOpts) (sshx.Client, error) {
		return fc, nil
	}
	m := New(8, dial, nil)
	waitDone(t, m.ConnectBatch(context.Background(), []Target{{SessionID: "x", Name: "x", Host: "h"}}, nil))

	act := m.Active()
	if len(act) != 1 || act[0].State != StateConnected {
		t.Fatalf("active = %+v, want one connected", act)
	}
	m.Disconnect("x")
	if len(m.Active()) != 0 {
		t.Fatalf("disconnect did not remove entry")
	}
	if !fc.closed.Load() {
		t.Fatalf("client not closed on disconnect")
	}
}

func TestDialError(t *testing.T) {
	dial := func(_ context.Context, _ string, _ int, _ string, _ sshx.Prompter, _ sshx.DialOpts) (sshx.Client, error) {
		return nil, errors.New("host key mismatch")
	}
	m := New(8, dial, nil)
	waitDone(t, m.ConnectBatch(context.Background(), []Target{{SessionID: "y", Name: "y", Host: "h"}}, nil))
	act := m.Active()
	if len(act) != 1 || act[0].State != StateError || act[0].Err == "" {
		t.Fatalf("active = %+v, want one error row", act)
	}
}

func TestSkipAlreadyConnected(t *testing.T) {
	var calls atomic.Int32
	dial := func(_ context.Context, _ string, _ int, _ string, _ sshx.Prompter, _ sshx.DialOpts) (sshx.Client, error) {
		calls.Add(1)
		return &fakeClient{}, nil
	}
	m := New(8, dial, nil)
	tg := []Target{{SessionID: "z", Name: "z", Host: "h"}}
	waitDone(t, m.ConnectBatch(context.Background(), tg, nil))
	waitDone(t, m.ConnectBatch(context.Background(), tg, nil)) // second should skip
	if calls.Load() != 1 {
		t.Fatalf("dial calls = %d, want 1 (second batch must skip connected)", calls.Load())
	}
}

func TestDisconnectAll(t *testing.T) {
	clients := make([]*fakeClient, 0, 5)
	var mu sync.Mutex
	dial := func(_ context.Context, _ string, _ int, _ string, _ sshx.Prompter, _ sshx.DialOpts) (sshx.Client, error) {
		fc := &fakeClient{}
		mu.Lock()
		clients = append(clients, fc)
		mu.Unlock()
		return fc, nil
	}
	m := New(8, dial, nil)
	waitDone(t, m.ConnectBatch(context.Background(), targets(5), nil))
	m.DisconnectAll()
	if len(m.Active()) != 0 {
		t.Fatal("DisconnectAll left entries")
	}
	for _, c := range clients {
		if !c.closed.Load() {
			t.Fatal("DisconnectAll did not close a client")
		}
	}
}

func TestWatchClosesDeadClient(t *testing.T) {
	fc := &fakeClient{}
	dial := func(_ context.Context, _ string, _ int, _ string, _ sshx.Prompter, _ sshx.DialOpts) (sshx.Client, error) {
		return fc, nil
	}
	m := New(0, dial, func() {})
	<-m.ConnectBatch(context.Background(), []Target{{SessionID: "s1", Host: "h"}}, nil)
	fc.die() // transport death WITHOUT a Close: only watch can set closed
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !fc.closed.Load() {
		time.Sleep(5 * time.Millisecond)
	}
	if !fc.closed.Load() {
		t.Fatal("watch must Close() the dead client so local resources (SOCKS listener) are released")
	}
}
