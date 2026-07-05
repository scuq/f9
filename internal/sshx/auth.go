package sshx

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
)

// buildAuth assembles the ordered auth chain: agent (if available), key files
// (lazy, passphrase via Prompter), password callback, keyboard-interactive.
// Nothing is persisted (ADR-0005); prompts fire only if the server offers the
// method and earlier methods failed.
func buildAuth(user, host string, keyFiles []string, useAgent bool, p Prompter) []ssh.AuthMethod {
	var methods []ssh.AuthMethod
	if useAgent {
		if sig := agentSigners(); sig != nil {
			methods = append(methods, ssh.PublicKeysCallback(sig))
		}
	}
	methods = append(methods, ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
		return loadKeySigners(keyFiles, p)
	}))
	if p != nil {
		methods = append(methods,
			ssh.PasswordCallback(func() (string, error) { return p.Password(user, host) }),
			ssh.KeyboardInteractive(p.KeyboardInteractive),
		)
	}
	return methods
}

// loadKeySigners parses the given key files. Encrypted keys prompt for a
// passphrase via p; unreadable or undecryptable keys are skipped (auth simply
// proceeds without them) rather than failing the whole method.
func loadKeySigners(keyFiles []string, p Prompter) ([]ssh.Signer, error) {
	var signers []ssh.Signer
	for _, path := range keyFiles {
		pemBytes, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		signer, err := ssh.ParsePrivateKey(pemBytes)
		if err == nil {
			signers = append(signers, signer)
			continue
		}
		var missing *ssh.PassphraseMissingError
		if !errors.As(err, &missing) || p == nil {
			continue
		}
		phrase, perr := p.Passphrase(path)
		if perr != nil || phrase == "" {
			continue
		}
		signer, err = ssh.ParsePrivateKeyWithPassphrase(pemBytes, []byte(phrase))
		if err != nil {
			continue
		}
		signers = append(signers, signer)
	}
	return signers, nil
}

// defaultKeyFiles returns the standard key paths that actually exist.
func defaultKeyFiles() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var out []string
	for _, name := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
		path := filepath.Join(home, ".ssh", name)
		if _, err := os.Stat(path); err == nil {
			out = append(out, path)
		}
	}
	return out
}
