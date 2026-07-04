// Package multisend sends a line or snippet to N marked sessions and tracks a
// per-session feedback state machine: sent -> echoed -> prompt-returned ->
// ok|error-pattern|timeout. Prompt/error regexes come from osdetect tuning
// profiles — passive matching only (no bytes injected). Phase 06.
package multisend

import (
	"bytes"
	"regexp"
	"strings"
	"sync"
	"time"
)

type State string

const (
	StPending        State = "pending" // sequential: not yet its turn
	StSent           State = "sent"
	StEchoed         State = "echoed"
	StPromptReturned State = "prompt-returned"
	StOK             State = "ok"
	StError          State = "error"
	StTimeout        State = "timeout"
)

const maxTail = 8192 // captured output tail cap per target

// Result is a snapshot of one target's progress.
type Result struct {
	ID      string `json:"id"`
	State   State  `json:"state"`
	Line    string `json:"line"`
	Tail    string `json:"tail"`
	ErrText string `json:"errText"`
	Millis  int64  `json:"millis"`
}

// Target is one session's feedback state machine. Matching is passive: it reads
// the output stream and watches for the echoed command, an error pattern, and
// the returned prompt.
type Target struct {
	id       string
	promptRe *regexp.Regexp
	errorRe  *regexp.Regexp

	mu      sync.Mutex
	state   State
	line    string
	tail    []byte
	sawEcho bool
	errText string
	started time.Time
	ended   time.Time
}

// NewTarget creates a target. promptRe/errorRe may be nil (then that signal is
// never matched).
func NewTarget(id string, promptRe, errorRe *regexp.Regexp) *Target {
	return &Target{id: id, promptRe: promptRe, errorRe: errorRe, state: StPending}
}

func (t *Target) isFinal() bool {
	return t.state == StOK || t.state == StError || t.state == StTimeout
}

// Final reports whether the target reached a terminal state.
func (t *Target) Final() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.isFinal()
}

func (t *Target) begin(line string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.line = line
	t.tail = t.tail[:0]
	t.sawEcho = false
	t.errText = ""
	t.ended = time.Time{}
	t.started = now
	t.state = StSent
}

// feed processes an output chunk. Returns true if the observable state changed.
func (t *Target) feed(data []byte, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == StPending || t.isFinal() {
		return false
	}
	prev := t.state
	clean := stripANSI(data)
	t.tail = append(t.tail, clean...)
	if len(t.tail) > maxTail {
		t.tail = t.tail[len(t.tail)-maxTail:]
	}
	if !t.sawEcho && t.line != "" && bytes.Contains(t.tail, []byte(t.line)) {
		t.sawEcho = true
		if t.state == StSent {
			t.state = StEchoed
		}
	}
	if t.errText == "" && t.errorRe != nil {
		if loc := t.errorRe.FindIndex(t.tail); loc != nil {
			t.errText = strings.TrimSpace(string(lineAt(t.tail, loc[0])))
		}
	}
	// The command echo line never matches an anchored prompt regex, so a prompt
	// match on the last line means the device returned to its prompt.
	if t.promptRe != nil && t.promptRe.Match(lastLine(t.tail)) {
		if t.errText != "" {
			t.state = StError
		} else {
			t.state = StOK
		}
		t.ended = now
	}
	return t.state != prev
}

func (t *Target) markTimeout(now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == StPending || t.isFinal() {
		return false
	}
	t.state = StTimeout
	t.ended = now
	return true
}

func (t *Target) failSend(msg string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state = StError
	t.errText = msg
	t.ended = now
}

func (t *Target) elapsed(now time.Time) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.started.IsZero() {
		return 0
	}
	return now.Sub(t.started)
}

func (t *Target) snapshot() Result {
	t.mu.Lock()
	defer t.mu.Unlock()
	var ms int64
	if !t.started.IsZero() {
		end := t.ended
		if end.IsZero() {
			end = time.Now()
		}
		ms = end.Sub(t.started).Milliseconds()
	}
	return Result{ID: t.id, State: t.state, Line: t.line, Tail: string(t.tail), ErrText: t.errText, Millis: ms}
}

// SendFunc delivers the prepared line to a target's terminal. Returns an error
// if the write fails (the target is then marked errored).
type SendFunc func(id, line string) error

// Job coordinates a broadcast over an ordered set of targets.
type Job struct {
	targets  []*Target
	byID     map[string]*Target
	lines    map[string]string
	send     SendFunc
	seq      bool
	timeout  time.Duration
	onChange func(Result)

	mu      sync.Mutex
	cur     int // sequential active index
	started bool
}

// NewJob builds a job. lines maps target id -> the (already rendered) line to
// send. seq runs targets one at a time (next starts after the previous
// finalizes). onChange fires on every observable state transition; it must not
// call back into the job.
func NewJob(targets []*Target, lines map[string]string, send SendFunc, seq bool, timeout time.Duration, onChange func(Result)) *Job {
	byID := make(map[string]*Target, len(targets))
	for _, t := range targets {
		byID[t.id] = t
	}
	return &Job{targets: targets, byID: byID, lines: lines, send: send, seq: seq, timeout: timeout, onChange: onChange}
}

func (j *Job) fire(r Result) {
	if j.onChange != nil {
		j.onChange(r)
	}
}

func (j *Job) startTarget(t *Target, now time.Time) {
	line := j.lines[t.id]
	t.begin(line, now)
	j.fire(t.snapshot())
	if j.send == nil {
		return
	}
	if err := j.send(t.id, line); err != nil {
		t.failSend("send: "+err.Error(), now)
		j.fire(t.snapshot())
		if j.seq {
			j.advanceSeq(t, now)
		}
	}
}

// Start begins the broadcast: all targets at once, or the first (sequential).
func (j *Job) Start(now time.Time) {
	j.mu.Lock()
	if j.started {
		j.mu.Unlock()
		return
	}
	j.started = true
	j.mu.Unlock()

	if j.seq {
		if len(j.targets) > 0 {
			j.startTarget(j.targets[0], now)
		}
		return
	}
	for _, t := range j.targets {
		j.startTarget(t, now)
	}
}

// Feed routes an output chunk to a target; call from that terminal's onData.
func (j *Job) Feed(id string, data []byte, now time.Time) {
	t, ok := j.byID[id]
	if !ok {
		return
	}
	if t.feed(data, now) {
		j.fire(t.snapshot())
		if t.Final() && j.seq {
			j.advanceSeq(t, now)
		}
	}
}

// Sweep times out any running target whose deadline passed. Call periodically.
func (j *Job) Sweep(now time.Time) {
	for _, t := range j.targets {
		if t.elapsed(now) >= j.timeout && t.markTimeout(now) {
			j.fire(t.snapshot())
			if j.seq {
				j.advanceSeq(t, now)
			}
		}
	}
}

// advanceSeq starts the next target, but only if finished is the current one
// (guards against a double transition racing feed and sweep).
func (j *Job) advanceSeq(finished *Target, now time.Time) {
	j.mu.Lock()
	if j.cur >= len(j.targets) || j.targets[j.cur] != finished {
		j.mu.Unlock()
		return
	}
	j.cur++
	var nt *Target
	if j.cur < len(j.targets) {
		nt = j.targets[j.cur]
	}
	j.mu.Unlock()
	if nt != nil {
		j.startTarget(nt, now)
	}
}

// Results snapshots every target.
func (j *Job) Results() []Result {
	out := make([]Result, len(j.targets))
	for i, t := range j.targets {
		out[i] = t.snapshot()
	}
	return out
}

// Done reports whether every target reached a terminal state.
func (j *Job) Done() bool {
	for _, t := range j.targets {
		if !t.Final() {
			return false
		}
	}
	return true
}

// --- passive-matching helpers (self-contained) ---

func stripANSI(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c == 0x1b {
			if i+1 < len(b) && b[i+1] == '[' {
				j := i + 2
				for j < len(b) && (b[j] < 0x40 || b[j] > 0x7e) {
					j++
				}
				i = j
				continue
			}
			i++
			continue
		}
		if c == '\r' {
			continue
		}
		out = append(out, c)
	}
	return out
}

func lastLine(b []byte) []byte {
	if i := bytes.LastIndexByte(b, '\n'); i >= 0 {
		return b[i+1:]
	}
	return b
}

func lineAt(b []byte, idx int) []byte {
	if idx < 0 || idx > len(b) {
		return nil
	}
	start := bytes.LastIndexByte(b[:idx], '\n') + 1
	end := bytes.IndexByte(b[idx:], '\n')
	if end < 0 {
		return b[start:]
	}
	return b[start : idx+end]
}
