// Package sshx wraps golang.org/x/crypto/ssh: dialing with timeouts and
// keepalives, the ordered auth chain (agent -> key files -> interactive;
// ADR-0005: nothing persisted), TOFU host-key handling, and jump chains in
// both proxyjump and shell-hop modes. See docs/phase-plan.md 00c.
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
	// ConfirmHostKey implements TOFU: called for unknown host keys only.
	// Key mismatches are never confirmable — they fail hard.
	ConfirmHostKey(host, fingerprint string) (accept bool, err error)
}

// Hop is one hop of a jump chain. Mode "proxyjump" (TCP forward via the hop,
// default when empty) or "shell-hop" (interactive ssh launched on the hop; for
// bastions that forbid TCP forwarding). Empty User inherits the target user.
type Hop struct {
	Host string
	Port int
	User string
	Mode string
}

type DialOpts struct {
	Timeout           time.Duration // default 10s; also bounds each handshake
	KeepaliveInterval time.Duration // 0 = no keepalives
	JumpChain         []Hop         // applied in order

	KeyFiles       []string     // default: existing ~/.ssh/{id_ed25519,id_ecdsa,id_rsa}
	NoAgent        bool         // skip the ssh-agent (key files / password only)
	KnownHostsPath string       // default: ~/.config/f9/known_hosts
	OnBanner       func(string) // login banner tap (osdetect consumer, 00d)
}

// Session is one interactive channel. OnData fans out to the scrollback
// buffer, the terminal stream and (phase 06) the multi-send matcher. Data
// arriving before the first OnData registration is buffered (bounded) and
// replayed to the first handler.
type Session interface {
	Stdin() io.Writer
	OnData(func(p []byte))
	Resize(cols, rows int) error
	Wait() error
	Close() error
}

type Client interface {
	NewSession(ctx context.Context, termType string, cols, rows int) (Session, error)
	// ServerVersion returns the SSH server version string (osdetect evidence).
	// For shell-hop clients this is the hop's version, not the target's.
	ServerVersion() string
	Close() error
}

var (
	ErrHostKeyRejected = errors.New("sshx: host key rejected by user")
	ErrHostKeyMismatch = errors.New("sshx: host key mismatch (possible MITM)")
)
