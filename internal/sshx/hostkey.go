package sshx

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// tofu wraps a knownhosts callback with trust-on-first-use semantics:
// unknown host -> Prompter.ConfirmHostKey, accepted keys appended to f9's own
// known_hosts file; changed key -> ErrHostKeyMismatch, never a prompt.
type tofu struct {
	mu    sync.Mutex
	path  string
	inner ssh.HostKeyCallback
	p     Prompter
}

func newTOFU(path string, p Prompter) (*tofu, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("sshx: known_hosts dir: %w", err)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			return nil, fmt.Errorf("sshx: create known_hosts: %w", err)
		}
	}
	inner, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("sshx: load known_hosts: %w", err)
	}
	return &tofu{path: path, inner: inner, p: p}, nil
}

func (t *tofu) check(hostname string, remote net.Addr, key ssh.PublicKey) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	err := t.inner(hostname, remote, key)
	if err == nil {
		return nil
	}
	var kerr *knownhosts.KeyError
	if !errors.As(err, &kerr) {
		return err
	}
	if len(kerr.Want) > 0 {
		return fmt.Errorf("%w: %s offers %s", ErrHostKeyMismatch, hostname, ssh.FingerprintSHA256(key))
	}
	// Unknown host: TOFU.
	if t.p == nil {
		return ErrHostKeyRejected
	}
	ok, perr := t.p.ConfirmHostKey(hostname, ssh.FingerprintSHA256(key))
	if perr != nil {
		return perr
	}
	if !ok {
		return ErrHostKeyRejected
	}
	if err := t.appendKey(hostname, key); err != nil {
		return err
	}
	if inner, err := knownhosts.New(t.path); err == nil {
		t.inner = inner // subsequent hops in this chain see the new key
	}
	return nil
}

func (t *tofu) appendKey(hostname string, key ssh.PublicKey) error {
	f, err := os.OpenFile(t.path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("sshx: open known_hosts: %w", err)
	}
	defer f.Close()
	line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
	if _, err := f.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("sshx: append known_hosts: %w", err)
	}
	return nil
}
