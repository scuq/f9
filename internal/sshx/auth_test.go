package sshx

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
)

func writeKey(t *testing.T, dir, name string, passphrase string) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var block *pem.Block
	if passphrase == "" {
		block, err = ssh.MarshalPrivateKey(priv, "")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte(passphrase))
	}
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadKeySignersPlain(t *testing.T) {
	dir := t.TempDir()
	path := writeKey(t, dir, "id_test", "")
	signers, err := loadKeySigners([]string{path, filepath.Join(dir, "missing")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(signers) != 1 {
		t.Fatalf("signers = %d, want 1 (missing file skipped)", len(signers))
	}
}

type prompterWithPassphrase struct {
	*mockPrompter
	phrase string
}

func (p prompterWithPassphrase) Passphrase(string) (string, error) { return p.phrase, nil }

func TestLoadKeySignersEncrypted(t *testing.T) {
	dir := t.TempDir()
	path := writeKey(t, dir, "id_enc", "s3cret")
	base := &mockPrompter{password: "unused"}

	signers, err := loadKeySigners([]string{path}, prompterWithPassphrase{base, "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	if len(signers) != 1 {
		t.Fatalf("signers with correct passphrase = %d, want 1", len(signers))
	}

	signers, err = loadKeySigners([]string{path}, prompterWithPassphrase{base, "wrong"})
	if err != nil {
		t.Fatal(err)
	}
	if len(signers) != 0 {
		t.Fatalf("signers with wrong passphrase = %d, want 0 (skipped)", len(signers))
	}
}

func TestShellHopCommand(t *testing.T) {
	cmd, err := shellHopCommand("10.21.194.1", 2222, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if cmd != "ssh -p 2222 admin@10.21.194.1" {
		t.Fatalf("cmd = %q", cmd)
	}
	cmd, err = shellHopCommand("sw00net050.kages.ad.local", 22, "")
	if err != nil {
		t.Fatal(err)
	}
	if cmd != "ssh sw00net050.kages.ad.local" {
		t.Fatalf("cmd = %q", cmd)
	}
	if _, err := shellHopCommand("host; rm -rf /", 22, "admin"); err == nil {
		t.Fatal("expected injection rejection for host")
	}
	if _, err := shellHopCommand("host", 22, "admin$(id)"); err == nil {
		t.Fatal("expected injection rejection for user")
	}
}
