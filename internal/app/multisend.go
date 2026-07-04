package app

import (
	"fmt"
	"regexp"
	"time"

	"github.com/scuq/f9/internal/multisend"
)

// MSPreview is a per-target dry-run row: the rendered line, the session's OS
// family (for mismatch guard rails), and any unresolved template variables.
type MSPreview struct {
	TermID     string   `json:"termId"`
	SessionID  string   `json:"sessionId"`
	Name       string   `json:"name"`
	Host       string   `json:"host"`
	OSFamily   string   `json:"osFamily"`
	Line       string   `json:"line"`
	Unresolved []string `json:"unresolved"`
	Err        string   `json:"err"`
}

// sessionRegexes resolves the prompt/error regexes for a session's OS family
// from the osdetect tuning profiles (nil when unknown — the target then relies
// on timeout).
func (a *App) sessionRegexes(sessionID string) (*regexp.Regexp, *regexp.Regexp) {
	fam := a.sessionFamily(sessionID)
	if fam == "" {
		return nil, nil
	}
	tun, ok := a.tunings[fam]
	if !ok {
		return nil, nil
	}
	var pr, er *regexp.Regexp
	if tun.PromptRegex != "" {
		if re, err := regexp.Compile(tun.PromptRegex); err == nil {
			pr = re
		}
	}
	if tun.ErrorRegex != "" {
		if re, err := regexp.Compile(tun.ErrorRegex); err == nil {
			er = re
		}
	}
	return pr, er
}

// MultiSendPreview renders body per target and reports OS family + unresolved
// vars — feeds the dry-run grid and the confirm dialog.
func (a *App) MultiSendPreview(termIDs []string, body string) []MSPreview {
	out := make([]MSPreview, 0, len(termIDs))
	for _, termID := range termIDs {
		a.tmu.Lock()
		t, ok := a.terms[termID]
		a.tmu.Unlock()
		if !ok {
			out = append(out, MSPreview{TermID: termID, Err: "terminal not open"})
			continue
		}
		p := MSPreview{TermID: termID, SessionID: t.sessionID}
		if s, _, err := a.st.Resolve(t.sessionID); err == nil {
			p.Name = s.Name
			p.Host = s.Host
		}
		p.OSFamily = string(a.sessionFamily(t.sessionID))
		if line, err := a.renderFor(t.sessionID, body, nil); err != nil {
			p.Err = err.Error()
		} else {
			p.Line = line
		}
		if u, err := a.TemplateUnresolved(t.sessionID, body); err == nil {
			p.Unresolved = u
		}
		out = append(out, p)
	}
	return out
}

// MultiSendStart broadcasts body to the given terminals. Each line is rendered
// per session (extra overlays prompt answers), prompt/error matching uses each
// session's OS tuning, and per-target state changes emit f9:multisend events.
func (a *App) MultiSendStart(termIDs []string, body string, extra map[string]string, sequential bool, timeoutMs int) error {
	if len(termIDs) == 0 {
		return fmt.Errorf("app: no targets")
	}
	a.msMu.Lock()
	busy := a.msJob != nil
	a.msMu.Unlock()
	if busy {
		return fmt.Errorf("app: a multi-send is already running")
	}

	targets := make([]*multisend.Target, 0, len(termIDs))
	lines := make(map[string]string, len(termIDs))
	for _, termID := range termIDs {
		a.tmu.Lock()
		t, ok := a.terms[termID]
		a.tmu.Unlock()
		if !ok {
			return fmt.Errorf("app: terminal %s not open", termID)
		}
		line, err := a.renderFor(t.sessionID, body, extra)
		if err != nil {
			return fmt.Errorf("app: render %s: %w", termID, err)
		}
		pr, er := a.sessionRegexes(t.sessionID)
		targets = append(targets, multisend.NewTarget(termID, pr, er))
		lines[termID] = line
	}

	if timeoutMs <= 0 {
		timeoutMs = 15000
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond

	send := func(id, line string) error {
		a.tmu.Lock()
		t, ok := a.terms[id]
		a.tmu.Unlock()
		if !ok {
			return fmt.Errorf("terminal closed")
		}
		_, err := t.session.Stdin().Write([]byte(line + "\r"))
		return err
	}
	onChange := func(r multisend.Result) { a.emitEvent("f9:multisend", r) }

	job := multisend.NewJob(targets, lines, send, sequential, timeout, onChange)
	cancel := make(chan struct{})
	a.msMu.Lock()
	a.msJob = job
	a.msCancel = cancel
	a.msMu.Unlock()

	job.Start(time.Now())
	go a.multiSendSweeper(job, cancel)
	return nil
}

func (a *App) multiSendSweeper(job *multisend.Job, cancel chan struct{}) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-cancel:
			return
		case <-ticker.C:
			job.Sweep(time.Now())
			if job.Done() {
				a.msMu.Lock()
				if a.msJob == job {
					a.msJob = nil
					a.msCancel = nil
				}
				a.msMu.Unlock()
				a.emitEvent("f9:multisenddone", nil)
				return
			}
		}
	}
}

// MultiSendCancel stops an in-flight broadcast; targets keep their last state.
func (a *App) MultiSendCancel() {
	a.msMu.Lock()
	cancel := a.msCancel
	a.msJob = nil
	a.msCancel = nil
	a.msMu.Unlock()
	if cancel != nil {
		close(cancel)
	}
	a.emitEvent("f9:multisenddone", nil)
}

// feedMultisend routes a terminal's output into an active broadcast job.
func (a *App) feedMultisend(termID string, data []byte) {
	a.msMu.RLock()
	job := a.msJob
	a.msMu.RUnlock()
	if job != nil {
		job.Feed(termID, data, time.Now())
	}
}
