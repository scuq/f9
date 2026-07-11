package sshx

import (
	"net"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

// activityConn wraps a net.Conn and records when data was last received. It
// backs the OpenSSH ServerAlive model: inbound data is proof of life, so only
// unanswered probes on a silent link mean the link is dead.
type activityConn struct {
	net.Conn
	lastRead atomic.Int64 // unix nanos of the last successful read
}

func newActivityConn(c net.Conn) *activityConn {
	a := &activityConn{Conn: c}
	a.lastRead.Store(time.Now().UnixNano())
	return a
}

func (a *activityConn) Read(p []byte) (int, error) {
	n, err := a.Conn.Read(p)
	if n > 0 {
		a.lastRead.Store(time.Now().UnixNano())
	}
	return n, err
}

// readSince reports whether any data arrived after t.
func (a *activityConn) readSince(t time.Time) bool {
	return time.Unix(0, a.lastRead.Load()).After(t)
}

const (
	// kaReplyTimeout bounds how long one probe waits for the server's reply.
	kaReplyTimeout = 5 * time.Second
	// kaMissMax is how many consecutive probes may go unanswered on a silent
	// link before the client is closed (OpenSSH: ServerAliveCountMax).
	kaMissMax = 2
)

// runKeepalive probes c every interval (OpenSSH: ServerAliveInterval). A probe
// that gets no reply within kaReplyTimeout is forgiven when act saw inbound
// data during the wait (link alive, server merely busy); kaMissMax consecutive
// real misses force-close the client so Wait() returns and the death path
// runs. act may be nil (probe-only mode); stop ends the loop.
func runKeepalive(c *ssh.Client, interval time.Duration, act *activityConn, stop <-chan struct{}) {
	t := time.NewTicker(interval)
	defer t.Stop()
	missed := 0
	for {
		select {
		case <-t.C:
			probeStart := time.Now()
			done := make(chan error, 1)
			go func() {
				// Blocks forever on a half-dead link (no RST after a VPN/wifi
				// drop); bounded by the select below, released by the Close.
				_, _, err := c.SendRequest("keepalive@openssh.com", true, nil)
				done <- err
			}()
			ok := false
			select {
			case err := <-done:
				ok = err == nil
			case <-time.After(kaReplyTimeout):
			case <-stop:
				return
			}
			if !ok && act != nil && act.readSince(probeStart) {
				ok = true // data arrived while we waited: the link is alive
			}
			if ok {
				missed = 0
				continue
			}
			missed++
			if missed >= kaMissMax {
				c.Close()
				return
			}
		case <-stop:
			return
		}
	}
}
