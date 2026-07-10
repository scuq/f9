// Package connmgr owns live SSH connections for the GUI: bounded concurrent
// dials, a per-connection state machine, and teardown. It holds sshx.Client
// handles but does no terminal I/O — that is phase 02. See phase-plan 01c.
package connmgr

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/scuq/f9/internal/sshx"
)

type State string

const (
	StateDialing   State = "dialing"
	StateConnected State = "connected"
	StateError     State = "error"
)

// Target is a resolved session ready to dial.
type Target struct {
	SessionID    string
	Name         string
	Host         string
	Port         int
	User         string
	JumpChain    []sshx.Hop
	Keepalive    time.Duration
	KeyFiles     []string
	NoAgent      bool
	AgentSockets []string
	SocksPort    int
}

// Conn is the UI-facing snapshot of one connection.
type Conn struct {
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	State     State  `json:"state"`
	Err       string `json:"err"`
	Since     string `json:"since"` // RFC3339; wire-friendly for Wails bindings
}

// DialFunc matches sshx.Dial; injectable so the manager is testable without
// a network or a real SSH server.
type DialFunc func(ctx context.Context, host string, port int, user string, p sshx.Prompter, o sshx.DialOpts) (sshx.Client, error)

type entry struct {
	conn   Conn
	client sshx.Client
	since  time.Time
}

type Manager struct {
	mu       sync.Mutex
	conns    map[string]*entry
	sem      chan struct{}
	dial     DialFunc
	onChange func()
}

// New builds a manager. cap<=0 defaults to 64; dial nil defaults to sshx.Dial;
// onChange nil is a no-op.
func New(cap int, dial DialFunc, onChange func()) *Manager {
	if cap <= 0 {
		cap = 64
	}
	if dial == nil {
		dial = sshx.Dial
	}
	if onChange == nil {
		onChange = func() {}
	}
	return &Manager{
		conns:    map[string]*entry{},
		sem:      make(chan struct{}, cap),
		dial:     dial,
		onChange: onChange,
	}
}

// ConnectBatch dials all targets concurrently (bounded by the semaphore),
// sharing one prompter across the batch. Targets already dialing/connected are
// skipped. Returns a channel closed when every dial in this batch settles.
func (m *Manager) ConnectBatch(ctx context.Context, targets []Target, p sshx.Prompter) <-chan struct{} {
	done := make(chan struct{})
	var wg sync.WaitGroup
	for _, t := range targets {
		m.mu.Lock()
		if e, ok := m.conns[t.SessionID]; ok && (e.conn.State == StateDialing || e.conn.State == StateConnected) {
			m.mu.Unlock()
			continue
		}
		now := time.Now()
		m.conns[t.SessionID] = &entry{
			conn: Conn{
				SessionID: t.SessionID, Name: t.Name, Host: t.Host,
				State: StateDialing, Since: now.Format(time.RFC3339),
			},
			since: now,
		}
		m.mu.Unlock()
		m.onChange()

		wg.Add(1)
		go func(t Target) {
			defer wg.Done()
			select {
			case m.sem <- struct{}{}:
			case <-ctx.Done():
				m.settle(t.SessionID, nil, ctx.Err())
				return
			}
			defer func() { <-m.sem }()

			client, err := m.dial(ctx, t.Host, t.Port, t.User, p, sshx.DialOpts{
				KeepaliveInterval: t.Keepalive,
				JumpChain:         t.JumpChain,
				KeyFiles:          t.KeyFiles,
				NoAgent:           t.NoAgent,
				AgentSockets:      t.AgentSockets,
				SocksPort:         t.SocksPort,
			})
			m.settle(t.SessionID, client, err)
		}(t)
	}
	go func() { wg.Wait(); close(done) }()
	return done
}

// settle records a dial result. If the entry vanished (disconnected mid-dial),
// any freshly-opened client is closed.
func (m *Manager) settle(id string, client sshx.Client, err error) {
	m.mu.Lock()
	e, ok := m.conns[id]
	if !ok {
		m.mu.Unlock()
		if client != nil {
			client.Close()
		}
		return
	}
	if err != nil {
		e.conn.State = StateError
		e.conn.Err = err.Error()
	} else {
		e.conn.State = StateConnected
		e.client = client
	}
	m.mu.Unlock()
	m.onChange()
	if err == nil && client != nil {
		go m.watch(id, client)
	}
}

// watch blocks until the client's connection closes (server death, keepalive
// failure) and then removes the entry, mirroring an explicit Disconnect. If the
// entry was already removed (user disconnect) or replaced, it does nothing.
func (m *Manager) watch(id string, client sshx.Client) {
	_ = client.Wait()
	m.mu.Lock()
	e, ok := m.conns[id]
	if !ok || e.client != client {
		m.mu.Unlock()
		return
	}
	delete(m.conns, id)
	m.mu.Unlock()
	m.onChange()
}

// Active returns a snapshot of all tracked connections, oldest first.
func (m *Manager) Active() []Conn {
	m.mu.Lock()
	defer m.mu.Unlock()
	ents := make([]*entry, 0, len(m.conns))
	for _, e := range m.conns {
		ents = append(ents, e)
	}
	sort.Slice(ents, func(i, j int) bool {
		if !ents[i].since.Equal(ents[j].since) {
			return ents[i].since.Before(ents[j].since)
		}
		return ents[i].conn.Name < ents[j].conn.Name
	})
	out := make([]Conn, len(ents))
	for i, e := range ents {
		out[i] = e.conn
	}
	return out
}

// Disconnect closes and removes one connection (works for error rows too:
// the row is always dismissed).
func (m *Manager) Disconnect(id string) {
	m.mu.Lock()
	e, ok := m.conns[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	client := e.client
	delete(m.conns, id)
	m.mu.Unlock()
	if client != nil {
		client.Close()
	}
	m.onChange()
}

// DisconnectAll closes and removes every connection.
func (m *Manager) DisconnectAll() {
	m.mu.Lock()
	clients := make([]sshx.Client, 0, len(m.conns))
	for _, e := range m.conns {
		if e.client != nil {
			clients = append(clients, e.client)
		}
	}
	m.conns = map[string]*entry{}
	m.mu.Unlock()
	for _, c := range clients {
		c.Close()
	}
	m.onChange()
}

// Client returns the live client for a session (phase 02 attach point).
func (m *Manager) Client(id string) (sshx.Client, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.conns[id]
	if !ok || e.conn.State != StateConnected {
		return nil, false
	}
	return e.client, true
}
