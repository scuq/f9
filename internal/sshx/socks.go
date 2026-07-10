package sshx

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
)

// sshDialer is the subset of *ssh.Client used to open forwarded connections.
type sshDialer interface {
	Dial(network, addr string) (net.Conn, error)
}

// socksProxy is a minimal local SOCKS5 (CONNECT, no auth) proxy that forwards
// accepted connections through an SSH client — the equivalent of `ssh -D`.
type socksProxy struct {
	ln     net.Listener
	dial   sshDialer
	wg     sync.WaitGroup
	once   sync.Once
	closed chan struct{}

	mu     sync.Mutex
	active map[net.Conn]struct{}
}

// startSocks binds 127.0.0.1:port (port 0 = ephemeral, for tests) and forwards
// accepted connections through d.
func startSocks(port int, d sshDialer) (*socksProxy, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return nil, fmt.Errorf("sshx: socks listen :%d: %w", port, err)
	}
	p := &socksProxy{ln: ln, dial: d, closed: make(chan struct{}), active: map[net.Conn]struct{}{}}
	p.wg.Add(1)
	go p.serve()
	return p, nil
}

// Addr is the bound address (useful when port 0 was requested).
func (p *socksProxy) Addr() net.Addr { return p.ln.Addr() }

func (p *socksProxy) serve() {
	defer p.wg.Done()
	for {
		c, err := p.ln.Accept()
		if err != nil {
			return // listener closed
		}
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.handle(c)
		}()
	}
}

// Close stops the listener and force-closes in-flight connections so their
// copy loops return immediately (a browser holding a connection open must not
// block teardown), then waits for the handlers to finish.
func (p *socksProxy) Close() error {
	var err error
	p.once.Do(func() {
		close(p.closed)
		err = p.ln.Close()
	})
	p.mu.Lock()
	for c := range p.active {
		_ = c.Close()
	}
	p.mu.Unlock()
	p.wg.Wait()
	return err
}

func (p *socksProxy) handle(c net.Conn) {
	p.mu.Lock()
	p.active[c] = struct{}{}
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.active, c)
		p.mu.Unlock()
		c.Close()
	}()
	target, err := socksHandshake(c)
	if err != nil {
		return
	}
	remote, err := p.dial.Dial("tcp", target)
	if err != nil {
		_, _ = c.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // general failure
		return
	}
	defer remote.Close()
	if _, err := c.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil { // success
		return
	}
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(remote, c); done <- struct{}{} }()
	go func() { _, _ = io.Copy(c, remote); done <- struct{}{} }()
	<-done
}

// socksHandshake performs the SOCKS5 greeting + CONNECT request, returning the
// target "host:port". Only no-auth CONNECT is supported.
func socksHandshake(c net.Conn) (string, error) {
	buf := make([]byte, 262)
	if _, err := io.ReadFull(c, buf[:2]); err != nil {
		return "", err
	}
	if buf[0] != 0x05 {
		return "", fmt.Errorf("socks: not version 5")
	}
	n := int(buf[1])
	if n > 0 {
		if _, err := io.ReadFull(c, buf[:n]); err != nil {
			return "", err
		}
	}
	if _, err := c.Write([]byte{0x05, 0x00}); err != nil { // method: no auth
		return "", err
	}
	if _, err := io.ReadFull(c, buf[:4]); err != nil {
		return "", err
	}
	if buf[0] != 0x05 || buf[1] != 0x01 { // only CONNECT
		_, _ = c.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return "", fmt.Errorf("socks: unsupported command %d", buf[1])
	}
	var host string
	switch buf[3] {
	case 0x01: // IPv4
		if _, err := io.ReadFull(c, buf[:4]); err != nil {
			return "", err
		}
		host = net.IP(buf[:4]).String()
	case 0x03: // domain name
		if _, err := io.ReadFull(c, buf[:1]); err != nil {
			return "", err
		}
		l := int(buf[0])
		if _, err := io.ReadFull(c, buf[:l]); err != nil {
			return "", err
		}
		host = string(buf[:l])
	case 0x04: // IPv6
		if _, err := io.ReadFull(c, buf[:16]); err != nil {
			return "", err
		}
		host = net.IP(buf[:16]).String()
	default:
		return "", fmt.Errorf("socks: unknown address type %d", buf[3])
	}
	if _, err := io.ReadFull(c, buf[:2]); err != nil {
		return "", err
	}
	port := binary.BigEndian.Uint16(buf[:2])
	return net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}
