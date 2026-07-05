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

func TestAgentEndpoints(t *testing.T) {
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

	eps := AgentEndpoints([]string{sock})
	if len(eps) != 1 || !eps[0].Available || len(eps[0].Keys) != 1 || eps[0].Keys[0].Comment != "test@f9" {
		t.Fatalf("endpoints = %+v", eps)
	}

	bad := AgentEndpoints([]string{filepath.Join(dir, "nope.sock")})
	if len(bad) != 1 || bad[0].Available || bad[0].Error == "" {
		t.Fatalf("bad endpoint = %+v", bad)
	}
}

func TestResolveAgentSockets(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/env/sock")
	if got := resolveAgentSockets([]string{"/a", "", "/b"}); len(got) != 2 || got[0] != "/a" || got[1] != "/b" {
		t.Fatalf("configured wins: %+v", got)
	}
	if got := resolveAgentSockets(nil); len(got) != 1 || got[0] != "/env/sock" {
		t.Fatalf("env fallback: %+v", got)
	}
	t.Setenv("SSH_AUTH_SOCK", "")
	if got := resolveAgentSockets(nil); got != nil {
		t.Fatalf("no agent: %+v", got)
	}
}
