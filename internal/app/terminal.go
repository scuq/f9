package app

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/scuq/f9/internal/osdetect"
	"github.com/scuq/f9/internal/scrollback"
	"github.com/scuq/f9/internal/sshx"
)

// terminal wraps one interactive SSH channel. Output feeds three consumers:
// the frontend stream, read-only activity detection, and the scrollback buffer
// that backs search / virtual grep (ANSI-stripped so grep sees clean lines).
type terminal struct {
	sessionID string
	session   sshx.Session
	sb        scrollback.Buffer

	closing atomic.Bool

	mu       sync.Mutex
	promptRe *regexp.Regexp
	watchRe  *regexp.Regexp
	running  bool
	tail     []byte
	lastOut  time.Time
	// mark is the absolute scrollback line where the output of the most
	// recent command starts (the line after the one Enter was typed on).
	// typed records a non-whitespace keystroke since the previous Enter, so
	// a bare Enter (fresh prompt) or Space+Enter (paging) leaves mark alone.
	mark  int
	typed bool

	// Output flow control (see waitCredit): bytes emitted to the frontend
	// but not yet acknowledged via TermAck.
	flowMu   sync.Mutex
	flowCond *sync.Cond
	inflight int64
}

const outputThrottle = 250 * time.Millisecond

// Output flow control. The Wails event path never blocks in Go (on Linux it
// appends to an unbounded main-thread dispatch queue) and xterm.js queues
// writes without limit, so a flooding session (cat of a large file, yes,
// a long paste echoing back) grew memory in both processes until the WebView
// died. The frontend now acks consumed bytes; once more than flowWindow is
// unacked the SSH reader blocks, which stalls the remote via the SSH window —
// the same way a slow real terminal does. flowStall bounds the wait so a
// reloaded/detached frontend cannot wedge the session forever.
const (
	flowWindow = 4 << 20
	flowStall  = 30 * time.Second
)

// Viewport scrollback. xterm.js keeps its own line buffer in the WebView
// (about 12 bytes per cell: 200 columns x 80k lines is ~190 MiB), separate
// from the Go ring that backs search and holds the full history. The viewport
// follows the session's scrollbackLines option so it can be tuned per folder,
// clamped so a ring-sized value (5,000,000) does not allocate gigabytes in
// the browser.
const (
	viewportScrollbackDefault = 80_000
	viewportScrollbackMax     = 500_000
)

// viewportScrollback maps the resolved scrollbackLines option (0 = unset)
// to the xterm.js scrollback line count.
func viewportScrollback(lines int) int {
	if lines <= 0 {
		return viewportScrollbackDefault
	}
	if lines > viewportScrollbackMax {
		return viewportScrollbackMax
	}
	return lines
}

// TermScrollback returns the xterm.js scrollback (in lines) for a session's
// terminal viewport.
func (a *App) TermScrollback(sessionID string) int {
	return viewportScrollback(a.scrollbackLines(sessionID))
}

func (t *terminal) waitCredit(n int) {
	t.flowMu.Lock()
	defer t.flowMu.Unlock()
	if t.flowCond == nil {
		t.flowCond = sync.NewCond(&t.flowMu)
	}
	deadline := time.Now().Add(flowStall)
	for t.inflight > flowWindow && !t.closing.Load() {
		if time.Now().After(deadline) {
			t.inflight = 0 // frontend gone quiet; stop holding the link hostage
			break
		}
		timer := time.AfterFunc(time.Second, func() { t.flowMu.Lock(); t.flowCond.Broadcast(); t.flowMu.Unlock() })
		t.flowCond.Wait()
		timer.Stop()
	}
	t.inflight += int64(n)
}

func (t *terminal) ack(n int) {
	t.flowMu.Lock()
	t.inflight -= int64(n)
	if t.inflight < 0 {
		t.inflight = 0
	}
	if t.flowCond != nil {
		t.flowCond.Broadcast()
	}
	t.flowMu.Unlock()
}

func (t *terminal) wakeFlow() {
	t.flowMu.Lock()
	if t.flowCond != nil {
		t.flowCond.Broadcast()
	}
	t.flowMu.Unlock()
}

// TermAck reports bytes the frontend has consumed (written into xterm.js).
func (a *App) TermAck(termID string, n int) {
	a.tmu.Lock()
	t, ok := a.terms[termID]
	a.tmu.Unlock()
	if ok && n > 0 {
		t.ack(n)
	}
}

//go:embed os-tunings.yaml
var embeddedTunings []byte

func loadTunings() map[osdetect.Family]osdetect.Tuning {
	paths := []string{"configs/os-tunings.yaml"}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "f9", "os-tunings.yaml"))
	}
	for _, p := range paths {
		if t, err := osdetect.LoadTunings(p); err == nil && len(t) > 0 {
			return t
		}
	}
	if t, err := osdetect.ParseTunings(embeddedTunings); err == nil && len(t) > 0 {
		return t
	}
	return map[osdetect.Family]osdetect.Tuning{}
}

func (a *App) sessionFamily(sessionID string) osdetect.Family {
	if m, err := a.st.Meta(sessionID); err == nil && m.DetectedOS != "" {
		return osdetect.Family(m.DetectedOS)
	}
	return ""
}

// scrollbackLines returns the resolved per-session scrollback cap (0 -> the
// scrollback package default of 5,000,000).
func (a *App) scrollbackLines(sessionID string) int {
	if _, eff, err := a.st.Resolve(sessionID); err == nil && eff.ScrollbackLines != nil {
		return *eff.ScrollbackLines
	}
	return 0
}

func (a *App) OpenTerminal(termID, sessionID string, cols, rows int) error {
	if termID == "" {
		return fmt.Errorf("app: termID required")
	}
	a.tmu.Lock()
	if _, ok := a.terms[termID]; ok {
		a.tmu.Unlock()
		return nil
	}
	a.tmu.Unlock()

	client, ok := a.mgr.Client(sessionID)
	if !ok {
		return fmt.Errorf("app: session not connected")
	}
	relay := a.detectorRelay(sessionID)
	sv := client.ServerVersion()
	if relay {
		sv = "" // the banner belongs to the hop, not the target
	}
	a.ensureDetector(sessionID, sv, relay)
	sess, err := client.NewSession(context.Background(), "xterm-256color", cols, rows)
	if err != nil {
		return err
	}

	t := &terminal{sessionID: sessionID, session: sess}
	t.sb = scrollback.New(scrollback.Config{MaxLines: a.scrollbackLines(sessionID)})
	if fam := a.sessionFamily(sessionID); fam != "" {
		if tun, ok := a.tunings[fam]; ok && tun.PromptRegex != "" {
			if re, err := regexp.Compile(tun.PromptRegex); err == nil {
				t.promptRe = re
			}
		}
	}

	a.tmu.Lock()
	a.terms[termID] = t
	a.termSess[termID] = sessionID
	a.tmu.Unlock()

	a.rebalanceScrollback()

	dataEvent := "f9:term:" + termID
	sess.OnData(func(p []byte) {
		// Scrollback and classifiers first: they are bounded and must see
		// everything even while the frontend is being throttled.
		t.sb.Append(stripANSI(p))
		a.feedMultisend(termID, p)
		a.detectActivity(termID, t, p)
		a.observeOS(sessionID, p)
		t.waitCredit(len(p))
		a.emitEvent(dataEvent, base64.StdEncoding.EncodeToString(p))
	})
	go func() {
		_ = sess.Wait()
		a.tmu.Lock()
		delete(a.terms, termID)
		a.tmu.Unlock()
		t.wakeFlow()
		a.rebalanceScrollback()
		a.dropDetectorIfIdle(sessionID)
		t.sb.Close()
		a.emitEvent("f9:termclosed", map[string]interface{}{"termId": termID, "died": !t.closing.Load()})
	}()
	return nil
}

// detectActivity runs the read-only classifiers and emits f9:termactivity.
func (a *App) detectActivity(termID string, t *terminal, p []byte) {
	t.mu.Lock()
	now := time.Now()
	emitOutput := now.Sub(t.lastOut) > outputThrottle
	if emitOutput {
		t.lastOut = now
	}
	t.tail = append(t.tail, p...)
	if len(t.tail) > 4096 {
		t.tail = t.tail[len(t.tail)-4096:]
	}
	emitPrompt := false
	if t.promptRe != nil && t.running {
		if t.promptRe.Match(stripANSI(lastLine(t.tail))) {
			t.running = false
			emitPrompt = true
		}
	}
	emitMatch := t.watchRe != nil && t.watchRe.Match(p)
	t.mu.Unlock()

	if emitOutput {
		a.emitActivity(termID, "output")
	}
	if emitPrompt {
		a.emitActivity(termID, "prompt")
	}
	if emitMatch {
		a.emitActivity(termID, "match")
	}
}

func (a *App) emitActivity(termID, kind string) {
	a.emitEvent("f9:termactivity", map[string]string{"termId": termID, "kind": kind})
}

func (a *App) TermInput(termID, data string) {
	a.tmu.Lock()
	t, ok := a.terms[termID]
	a.tmu.Unlock()
	if !ok {
		return
	}
	if strings.ContainsAny(data, "\r\n") {
		lines, _ := t.sb.Len()
		end := t.sb.FirstLine() + lines // the partial prompt+command line is end-1
		t.mu.Lock()
		t.running = true
		for i := 0; i < len(data); i++ {
			switch data[i] {
			case '\r', '\n':
				if t.typed {
					t.mark = end
					t.typed = false
				}
			case ' ', '\t':
			default:
				t.typed = true
			}
		}
		t.mu.Unlock()
	} else if strings.IndexFunc(data, func(r rune) bool { return r != ' ' && r != '\t' }) >= 0 {
		t.mu.Lock()
		t.typed = true
		t.mu.Unlock()
	}
	_, _ = t.session.Stdin().Write([]byte(data))
}

// lastOutputMax bounds what one TermLastOutput call hands to the clipboard:
// the text crosses the IPC bridge as a single JSON string.
const lastOutputMax = 64 << 20

// TermLastOutput returns the terminal's scrollback since the last command was
// sent (everything, if none was): the "copy last output" shortcut. A command
// is an Enter preceded by at least one non-whitespace keystroke since the
// previous Enter; bare Enters and Space+Enter (pager) do not count. A trailing
// line that matches the detected OS's prompt regex is dropped so the copied
// text ends with the command's output rather than the next prompt. ANSI is
// already stripped in the scrollback, so the text is plain.
func (a *App) TermLastOutput(termID string) (string, error) {
	a.tmu.Lock()
	t, ok := a.terms[termID]
	a.tmu.Unlock()
	if !ok {
		return "", fmt.Errorf("app: terminal not open")
	}
	t.mu.Lock()
	from, promptRe := t.mark, t.promptRe
	t.mu.Unlock()
	lines, size := t.sb.Len()
	first := t.sb.FirstLine()
	end := first + lines
	if from < first {
		from = first // evicted: start at the oldest retained line
	}
	if from >= end {
		return "", nil
	}
	if size > lastOutputMax {
		return "", fmt.Errorf("app: last output exceeds %d MiB; use search to extract what you need", lastOutputMax>>20)
	}
	out, err := t.sb.Lines(from, end)
	if err != nil {
		return "", err
	}
	if n := len(out); n > 0 && promptRe != nil && promptRe.Match(out[n-1]) {
		out = out[:n-1]
	}
	var sb strings.Builder
	for i, ln := range out {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.Write(ln)
	}
	return sb.String(), nil
}

func (a *App) TermResize(termID string, cols, rows int) {
	a.tmu.Lock()
	t, ok := a.terms[termID]
	a.tmu.Unlock()
	if ok {
		_ = t.session.Resize(cols, rows)
	}
}

// Closing the last terminal of a session also drops the underlying
// connection; other terminals on the same session keep it alive.
func (a *App) CloseTerminal(termID string) {
	a.tmu.Lock()
	t, ok := a.terms[termID]
	delete(a.terms, termID)
	sessionID, tracked := a.termSess[termID]
	delete(a.termSess, termID)
	last := tracked
	for _, sid := range a.termSess {
		if sid == sessionID {
			last = false
			break
		}
	}
	a.tmu.Unlock()
	if ok {
		t.closing.Store(true)
		t.wakeFlow()
		_ = t.session.Close()
		t.sb.Close()
	}
	if last {
		a.mgr.Disconnect(sessionID)
	}
}

// SetTerminalWatch sets (or clears, with "") the per-tab watch regex.
func (a *App) SetTerminalWatch(termID, pattern string) error {
	a.tmu.Lock()
	t, ok := a.terms[termID]
	a.tmu.Unlock()
	if !ok {
		return fmt.Errorf("app: terminal not open")
	}
	var re *regexp.Regexp
	if pattern != "" {
		var err error
		re, err = regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("app: watch regex: %w", err)
		}
	}
	t.mu.Lock()
	t.watchRe = re
	t.mu.Unlock()
	return nil
}

func lastLine(b []byte) []byte {
	if i := bytes.LastIndexByte(b, '\n'); i >= 0 {
		return b[i+1:]
	}
	return b
}

// stripANSI removes CSI escape sequences and carriage returns so prompt
// detection and scrollback grep operate on clean text.
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
