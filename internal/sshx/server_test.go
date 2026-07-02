package sshx

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"

	"golang.org/x/crypto/ssh"
)

// testServer is a minimal in-process SSH server: password auth ("pw"),
// echoing session channels, and direct-tcpip forwarding (for proxyjump tests).
type testServer struct {
	addr   string
	signer ssh.Signer
}

func startTestServer(t *testing.T) *testServer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if string(pw) == "pw" {
				return nil, nil
			}
			return nil, errors.New("denied")
		},
	}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go handleServerConn(c, cfg)
		}
	}()
	return &testServer{addr: ln.Addr().String(), signer: signer}
}

func handleServerConn(c net.Conn, cfg *ssh.ServerConfig) {
	sconn, chans, reqs, err := ssh.NewServerConn(c, cfg)
	if err != nil {
		c.Close()
		return
	}
	defer sconn.Close()
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		switch nc.ChannelType() {
		case "session":
			ch, creqs, err := nc.Accept()
			if err != nil {
				continue
			}
			go func() {
				for r := range creqs {
					ok := r.Type == "pty-req" || r.Type == "shell" ||
						r.Type == "window-change" || r.Type == "env"
					if r.WantReply {
						r.Reply(ok, nil)
					}
				}
			}()
			go func() {
				io.Copy(ch, ch) // echo
				ch.Close()
			}()
		case "direct-tcpip":
			var d struct {
				DestAddr string
				DestPort uint32
				OrigAddr string
				OrigPort uint32
			}
			if err := ssh.Unmarshal(nc.ExtraData(), &d); err != nil {
				nc.Reject(ssh.ConnectionFailed, "bad payload")
				continue
			}
			dst, err := net.Dial("tcp", net.JoinHostPort(d.DestAddr, fmt.Sprint(d.DestPort)))
			if err != nil {
				nc.Reject(ssh.ConnectionFailed, err.Error())
				continue
			}
			ch, _, err := nc.Accept()
			if err != nil {
				dst.Close()
				continue
			}
			go func() {
				io.Copy(dst, ch)
				dst.Close()
			}()
			go func() {
				io.Copy(ch, dst)
				ch.Close()
			}()
		default:
			nc.Reject(ssh.UnknownChannelType, "")
		}
	}
}

// mockPrompter answers everything with a fixed password and records TOFU calls.
type mockPrompter struct {
	password     string
	accept       bool
	confirmCalls int
}

func (m *mockPrompter) Passphrase(string) (string, error) { return "", errors.New("none") }
func (m *mockPrompter) Password(user, host string) (string, error) {
	return m.password, nil
}
func (m *mockPrompter) KeyboardInteractive(name, instr string, qs []string, echos []bool) ([]string, error) {
	ans := make([]string, len(qs))
	for i := range ans {
		ans[i] = m.password
	}
	return ans, nil
}
func (m *mockPrompter) ConfirmHostKey(host, fp string) (bool, error) {
	m.confirmCalls++
	return m.accept, nil
}
