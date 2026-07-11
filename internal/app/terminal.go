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
}

const outputThrottle = 250 * time.Millisecond

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
	a.ensureDetector(sessionID, client.ServerVersion())
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
	a.tmu.Unlock()

	dataEvent := "f9:term:" + termID
	sess.OnData(func(p []byte) {
		a.emitEvent(dataEvent, base64.StdEncoding.EncodeToString(p))
		t.sb.Append(stripANSI(p))
		a.feedMultisend(termID, p)
		a.detectActivity(termID, t, p)
		a.observeOS(sessionID, p)
	})
	go func() {
		_ = sess.Wait()
		a.tmu.Lock()
		delete(a.terms, termID)
		a.tmu.Unlock()
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
		t.mu.Lock()
		t.running = true
		t.mu.Unlock()
	}
	_, _ = t.session.Stdin().Write([]byte(data))
}

func (a *App) TermResize(termID string, cols, rows int) {
	a.tmu.Lock()
	t, ok := a.terms[termID]
	a.tmu.Unlock()
	if ok {
		_ = t.session.Resize(cols, rows)
	}
}

func (a *App) CloseTerminal(termID string) {
	a.tmu.Lock()
	t, ok := a.terms[termID]
	delete(a.terms, termID)
	a.tmu.Unlock()
	if ok {
		t.closing.Store(true)
		_ = t.session.Close()
		t.sb.Close()
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
