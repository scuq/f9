//go:build !windows

package sshx

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh/agent"
)

func TestAgentAvailableAndKeys(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	keyring := agent.NewKeyring()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := keyring.Add(agent.AddedKey{PrivateKey: priv, Comment: "test@f9"}); err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { _ = agent.ServeAgent(keyring, c) }()
		}
	}()

	t.Setenv("SSH_AUTH_SOCK", sock)

	avail, gotSock := AgentAvailable()
	if !avail || gotSock != sock {
		t.Fatalf("AgentAvailable() = %v, %q", avail, gotSock)
	}
	keys, err := AgentKeys()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].Comment != "test@f9" || keys[0].Fingerprint == "" {
		t.Fatalf("keys = %+v", keys)
	}
}

func TestAgentUnavailable(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	if avail, _ := AgentAvailable(); avail {
		t.Fatal("expected unavailable with empty SSH_AUTH_SOCK")
	}
	keys, err := AgentKeys()
	if err != nil || keys != nil {
		t.Fatalf("keys=%v err=%v", keys, err)
	}
}
