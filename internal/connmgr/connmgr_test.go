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

type fakeClient struct{ closed atomic.Bool }

func (f *fakeClient) NewSession(_ context.Context, _ string, _, _ int) (sshx.Session, error) {
	return nil, nil
}
func (f *fakeClient) ServerVersion() string { return "SSH-2.0-fake" }
func (f *fakeClient) Close() error          { f.closed.Store(true); return nil }

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
