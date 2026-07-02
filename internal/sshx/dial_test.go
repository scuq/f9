package sshx

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh/knownhosts"
)

func splitAddr(t *testing.T, addr string) (string, int) {
	t.Helper()
	h, ps, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	p, err := strconv.Atoi(ps)
	if err != nil {
		t.Fatal(err)
	}
	return h, p
}

func waitForOutput(t *testing.T, ch <-chan []byte, want string) {
	t.Helper()
	var got strings.Builder
	deadline := time.After(3 * time.Second)
	for {
		select {
		case b := <-ch:
			got.Write(b)
			if strings.Contains(got.String(), want) {
				return
			}
		case <-deadline:
			t.Fatalf("timeout waiting for %q, got %q", want, got.String())
		}
	}
}

func TestDialTOFUAndSessionEcho(t *testing.T) {
	srv := startTestServer(t)
	host, port := splitAddr(t, srv.addr)
	kh := filepath.Join(t.TempDir(), "known_hosts")
	p := &mockPrompter{password: "pw", accept: true}

	c, err := Dial(context.Background(), host, port, "scuq", p,
		DialOpts{Timeout: 5 * time.Second, KnownHostsPath: kh})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	if p.confirmCalls != 1 {
		t.Fatalf("TOFU confirms = %d, want 1", p.confirmCalls)
	}
	if !strings.HasPrefix(c.ServerVersion(), "SSH-2.0") {
		t.Fatalf("ServerVersion = %q", c.ServerVersion())
	}
	data, err := os.ReadFile(kh)
	if err != nil || !strings.Contains(string(data), "ssh-ed25519") {
		t.Fatalf("known_hosts not persisted: %v / %q", err, data)
	}

	s, err := c.NewSession(context.Background(), "xterm", 80, 24)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Close()

	out := make(chan []byte, 64)
	s.OnData(func(b []byte) { out <- append([]byte(nil), b...) })
	if _, err := s.Stdin().Write([]byte("hello f9\n")); err != nil {
		t.Fatal(err)
	}
	waitForOutput(t, out, "hello f9")

	if err := s.Resize(120, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}

	// Second dial: key is known now — no prompt.
	c2, err := Dial(context.Background(), host, port, "scuq", p,
		DialOpts{Timeout: 5 * time.Second, KnownHostsPath: kh})
	if err != nil {
		t.Fatalf("second Dial: %v", err)
	}
	c2.Close()
	if p.confirmCalls != 1 {
		t.Fatalf("TOFU confirms after second dial = %d, want still 1", p.confirmCalls)
	}
}

func TestHostKeyRejected(t *testing.T) {
	srv := startTestServer(t)
	host, port := splitAddr(t, srv.addr)
	kh := filepath.Join(t.TempDir(), "known_hosts")
	p := &mockPrompter{password: "pw", accept: false}

	_, err := Dial(context.Background(), host, port, "scuq", p,
		DialOpts{Timeout: 5 * time.Second, KnownHostsPath: kh})
	if !errors.Is(err, ErrHostKeyRejected) {
		t.Fatalf("err = %v, want ErrHostKeyRejected", err)
	}
}

func TestHostKeyMismatch(t *testing.T) {
	srv := startTestServer(t)
	host, port := splitAddr(t, srv.addr)
	other := startTestServer(t) // unrelated key

	kh := filepath.Join(t.TempDir(), "known_hosts")
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	line := knownhosts.Line([]string{knownhosts.Normalize(addr)}, other.signer.PublicKey())
	if err := os.WriteFile(kh, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	p := &mockPrompter{password: "pw", accept: true}
	_, err := Dial(context.Background(), host, port, "scuq", p,
		DialOpts{Timeout: 5 * time.Second, KnownHostsPath: kh})
	if !errors.Is(err, ErrHostKeyMismatch) {
		t.Fatalf("err = %v, want ErrHostKeyMismatch", err)
	}
	if p.confirmCalls != 0 {
		t.Fatalf("mismatch must never prompt; confirms = %d", p.confirmCalls)
	}
}

func TestProxyJump(t *testing.T) {
	hop := startTestServer(t)
	target := startTestServer(t)
	hopHost, hopPort := splitAddr(t, hop.addr)
	tgtHost, tgtPort := splitAddr(t, target.addr)

	kh := filepath.Join(t.TempDir(), "known_hosts")
	p := &mockPrompter{password: "pw", accept: true}

	c, err := Dial(context.Background(), tgtHost, tgtPort, "scuq", p, DialOpts{
		Timeout:        5 * time.Second,
		KnownHostsPath: kh,
		JumpChain:      []Hop{{Host: hopHost, Port: hopPort, Mode: "proxyjump"}},
	})
	if err != nil {
		t.Fatalf("Dial via jump: %v", err)
	}
	defer c.Close()

	if p.confirmCalls != 2 { // hop + target both unknown
		t.Fatalf("TOFU confirms = %d, want 2", p.confirmCalls)
	}

	s, err := c.NewSession(context.Background(), "xterm", 80, 24)
	if err != nil {
		t.Fatalf("NewSession via jump: %v", err)
	}
	defer s.Close()
	out := make(chan []byte, 64)
	s.OnData(func(b []byte) { out <- append([]byte(nil), b...) })
	if _, err := s.Stdin().Write([]byte("through the wormhole\n")); err != nil {
		t.Fatal(err)
	}
	waitForOutput(t, out, "through the wormhole")
}
