package app

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"
)

// waitCredit must block once flowWindow is exceeded, resume on ack, and
// never block a terminal that is closing.
func TestOutputFlowControl(t *testing.T) {
	tm := &terminal{}
	tm.waitCredit(flowWindow + 1) // over the window; next call must block
	done := make(chan struct{})
	go func() { tm.waitCredit(1); close(done) }()
	select {
	case <-done:
		t.Fatal("waitCredit did not block over the window")
	case <-time.After(50 * time.Millisecond):
	}
	tm.ack(flowWindow)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ack did not release waitCredit")
	}

	tm2 := &terminal{}
	tm2.waitCredit(flowWindow + 1)
	done2 := make(chan struct{})
	go func() { tm2.waitCredit(1); close(done2) }()
	time.Sleep(20 * time.Millisecond)
	tm2.closing.Store(true)
	tm2.wakeFlow()
	select {
	case <-done2:
	case <-time.After(2 * time.Second):
		t.Fatal("closing terminal stayed blocked in waitCredit")
	}
}

// A burst of tiny reads (Cisco IOS writes a line or less per SSH packet)
// must reach the frontend as a handful of ordered events, not one per read:
// each event is a separate JS evaluation in the WebView, and tens of
// thousands of them starved xterm's parser and with it the flow-control acks.
func TestOutputCoalescing(t *testing.T) {
	var mu sync.Mutex
	var events [][]byte
	tm := &terminal{emit: func(p []byte) {
		mu.Lock()
		events = append(events, append([]byte(nil), p...))
		mu.Unlock()
	}}

	tm.queueOutput([]byte("first"))
	mu.Lock()
	n := len(events)
	mu.Unlock()
	if n != 1 {
		t.Fatalf("first read after idle should be emitted immediately, got %d events", n)
	}

	const reads = 20000
	var want bytes.Buffer
	want.WriteString("first")
	for i := 0; i < reads; i++ {
		line := []byte(fmt.Sprintf("interface GigabitEthernet1/0/%d\r\n", i))
		want.Write(line)
		tm.queueOutput(line)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		total := 0
		for _, e := range events {
			total += len(e)
		}
		mu.Unlock()
		if total == want.Len() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("coalesced output never fully flushed: %d of %d bytes", total, want.Len())
		}
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(events) > reads/50 {
		t.Fatalf("%d reads became %d events; expected coalescing", reads, len(events))
	}
	if got := bytes.Join(events, nil); !bytes.Equal(got, want.Bytes()) {
		t.Fatal("coalesced stream differs from the input (order or content)")
	}
	for _, e := range events {
		if len(e) > outChunk+64 {
			t.Fatalf("event of %d bytes exceeds outChunk", len(e))
		}
	}

	// Idle again: the next read is immediate once more.
	time.Sleep(3 * outCoalesce)
	before := len(events)
	mu.Unlock()
	tm.queueOutput([]byte("prompt> "))
	mu.Lock()
	if len(events) != before+1 {
		t.Fatalf("read after idle not emitted immediately: %d -> %d events", before, len(events))
	}
}
