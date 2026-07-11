package sshx

import (
	"net"
	"testing"
	"time"
)

func TestActivityConnTracksReads(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	ac := newActivityConn(a)

	mark := time.Now()
	if ac.readSince(mark) {
		t.Fatal("no data yet: readSince(mark) must be false")
	}
	go func() { _, _ = b.Write([]byte("x")) }()
	buf := make([]byte, 1)
	if _, err := ac.Read(buf); err != nil {
		t.Fatal(err)
	}
	if !ac.readSince(mark) {
		t.Fatal("data arrived: readSince(mark) must be true")
	}
}
