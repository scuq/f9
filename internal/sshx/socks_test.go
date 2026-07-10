package sshx

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
)

type dialerFunc func(network, addr string) (net.Conn, error)

func (f dialerFunc) Dial(network, addr string) (net.Conn, error) { return f(network, addr) }

func TestSocksProxyForwards(t *testing.T) {
	// echo server standing in for the SSH-forwarded target
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(c, c); _ = c.Close() }()
		}
	}()

	var gotTarget string
	d := dialerFunc(func(network, addr string) (net.Conn, error) {
		gotTarget = addr
		return net.Dial("tcp", echo.Addr().String())
	})
	p, err := startSocks(0, d)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	c, err := net.Dial("tcp", p.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// greeting: v5, 1 method (no auth)
	if _, err := c.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatal(err)
	}
	rep := make([]byte, 2)
	if _, err := io.ReadFull(c, rep); err != nil || rep[0] != 0x05 || rep[1] != 0x00 {
		t.Fatalf("greeting reply %v err %v", rep, err)
	}
	// CONNECT example.com:8080 (domain)
	req := []byte{0x05, 0x01, 0x00, 0x03, byte(len("example.com"))}
	req = append(req, []byte("example.com")...)
	req = binary.BigEndian.AppendUint16(req, 8080)
	if _, err := c.Write(req); err != nil {
		t.Fatal(err)
	}
	rep2 := make([]byte, 10)
	if _, err := io.ReadFull(c, rep2); err != nil || rep2[1] != 0x00 {
		t.Fatalf("connect reply %v err %v", rep2, err)
	}
	if gotTarget != "example.com:8080" {
		t.Fatalf("target = %q", gotTarget)
	}
	// data path echoes
	if _, err := c.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	out := make([]byte, 4)
	if _, err := io.ReadFull(c, out); err != nil || string(out) != "ping" {
		t.Fatalf("echo = %q err %v", out, err)
	}
}

func TestSocksRejectsNonV5(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	go func() { _, _ = socksHandshake(server); server.Close() }()
	_, _ = client.Write([]byte{0x04, 0x01, 0x00})
	// handshake should fail (server closes); a read returns EOF/err
	buf := make([]byte, 1)
	if _, err := client.Read(buf); err == nil {
		t.Fatal("non-v5 greeting should be rejected")
	}
}
