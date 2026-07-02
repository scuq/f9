// Package sshx wraps golang.org/x/crypto/ssh: dialing with timeouts and
// keepalives, the ordered auth chain (agent -> key files -> interactive; ADR-0005:
// nothing persisted), TOFU host-key handling, and jump chains in both proxyjump
// and shell-hop modes. See docs/phase-plan.md 00c.
//
// Dependency note: this package gains golang.org/x/crypto/{ssh,ssh/agent,
// ssh/knownhosts} when 00c starts; the skeleton stays stdlib-only.
package sshx

import (
	"context"
	"errors"
	"io"
	"time"
)

// Prompter is implemented by the CLI (terminal prompts) and by the Wails UI
// (dialogs). Results are used once and never persisted.
type Prompter interface {
	Passphrase(keyPath string) (string, error)
	Password(user, host string) (string, error)
	KeyboardInteractive(name, instruction string, questions []string, echos []bool) ([]string, error)
	// ConfirmHostKey implements TOFU: called for unknown host keys.
	ConfirmHostKey(host, fingerprint string) (accept bool, err error)
}

type DialOpts struct {
	Timeout           time.Duration
	KeepaliveInterval time.Duration
	// JumpChain resolved from store options; applied in order.
	JumpChain []Hop
}

type Hop struct {
	Host, User, Mode string // Mode: proxyjump|shell-hop
	Port             int
}

// Session is one interactive channel. OnData fans out to the scrollback buffer,
// the terminal stream and (phase 06) the multi-send matcher.
type Session interface {
	Stdin() io.Writer
	OnData(func(p []byte)) // register fan-out consumer
	Resize(cols, rows int) error
	Wait() error
	Close() error
}

type Client interface {
	NewSession(ctx context.Context, termType string, cols, rows int) (Session, error)
	Close() error
}

var ErrHostKeyRejected = errors.New("sshx: host key rejected by user")

// Dial establishes the (possibly multi-hop) connection. Phase 00c.
func Dial(ctx context.Context, host string, port int, user string, p Prompter, o DialOpts) (Client, error) {
	panic("phase 00c: not implemented")
}
