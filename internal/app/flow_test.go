package app

import (
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
