package multisend

import (
	"regexp"
	"testing"
	"time"
)

var (
	promptRe = regexp.MustCompile(`[\w.\-@:~]+[#>$]\s*$`)
	errorRe  = regexp.MustCompile(`% [A-Z]`)
)

func TestTargetOK(t *testing.T) {
	tg := NewTarget("s1", promptRe, errorRe)
	now := time.Now()
	tg.begin("show clock", now)
	if tg.snapshot().State != StSent {
		t.Fatalf("want sent, got %s", tg.snapshot().State)
	}
	tg.feed([]byte("show clock\r\n"), now)
	if tg.snapshot().State != StEchoed {
		t.Fatalf("want echoed, got %s", tg.snapshot().State)
	}
	tg.feed([]byte("Sun Jul  5 00:42 UTC 2026\r\n"), now)
	tg.feed([]byte("switch# "), now)
	if r := tg.snapshot(); r.State != StOK {
		t.Fatalf("want ok, got %s", r.State)
	}
}

func TestTargetError(t *testing.T) {
	tg := NewTarget("s1", promptRe, errorRe)
	now := time.Now()
	tg.begin("shw clock", now)
	tg.feed([]byte("shw clock\r\n% Invalid input detected\r\nswitch# "), now)
	r := tg.snapshot()
	if r.State != StError {
		t.Fatalf("want error, got %s", r.State)
	}
	if r.ErrText == "" {
		t.Fatal("want captured errText")
	}
}

func TestTargetTimeout(t *testing.T) {
	tg := NewTarget("s1", promptRe, errorRe)
	start := time.Now()
	tg.begin("show clock", start)
	tg.feed([]byte("show clock\r\n"), start) // echoed, no prompt
	if !tg.markTimeout(start.Add(10 * time.Second)) {
		t.Fatal("want timeout transition")
	}
	if tg.snapshot().State != StTimeout {
		t.Fatal("want timeout state")
	}
}

func newTargets() []*Target {
	return []*Target{NewTarget("a", promptRe, errorRe), NewTarget("b", promptRe, errorRe)}
}

func TestJobParallel(t *testing.T) {
	var sent []string
	send := func(id, line string) error { sent = append(sent, id); return nil }
	targets := newTargets()
	lines := map[string]string{"a": "show clock", "b": "show clock"}
	j := NewJob(targets, lines, send, false, 5*time.Second, func(Result) {})
	now := time.Now()
	j.Start(now)
	if len(sent) != 2 {
		t.Fatalf("parallel should send all, sent %v", sent)
	}
	j.Feed("a", []byte("show clock\r\nout\r\nhost# "), now)
	j.Feed("b", []byte("show clock\r\nout\r\nhost# "), now)
	for _, r := range j.Results() {
		if r.State != StOK {
			t.Fatalf("%s want ok got %s", r.ID, r.State)
		}
	}
	if !j.Done() {
		t.Fatal("job should be done")
	}
}

func TestJobSequential(t *testing.T) {
	var sent []string
	send := func(id, line string) error { sent = append(sent, id); return nil }
	targets := newTargets()
	lines := map[string]string{"a": "show clock", "b": "show clock"}
	j := NewJob(targets, lines, send, true, 5*time.Second, func(Result) {})
	now := time.Now()
	j.Start(now)
	if len(sent) != 1 || sent[0] != "a" {
		t.Fatalf("sequential should send only a first, sent %v", sent)
	}
	j.Feed("a", []byte("show clock\r\nout\r\nhost# "), now)
	if len(sent) != 2 || sent[1] != "b" {
		t.Fatalf("b should send after a finalizes, sent %v", sent)
	}
	j.Feed("b", []byte("show clock\r\nout\r\nhost# "), now)
	if !j.Done() {
		t.Fatal("job should be done")
	}
}

func TestJobSeqTimeout(t *testing.T) {
	var sent []string
	send := func(id, line string) error { sent = append(sent, id); return nil }
	targets := newTargets()
	lines := map[string]string{"a": "x", "b": "x"}
	j := NewJob(targets, lines, send, true, 2*time.Second, func(Result) {})
	start := time.Now()
	j.Start(start)
	j.Sweep(start.Add(3 * time.Second)) // a hangs -> timeout -> b sends
	if len(sent) != 2 || sent[1] != "b" {
		t.Fatalf("b should send after a times out, sent %v", sent)
	}
	if targets[0].snapshot().State != StTimeout {
		t.Fatalf("a should be timeout, got %s", targets[0].snapshot().State)
	}
}
