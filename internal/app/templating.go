package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/scuq/f9/internal/snippets"
	"github.com/scuq/f9/internal/vars"
)

// VarsScopeDTO selects a vars scope from the frontend.
type VarsScopeDTO struct {
	FolderID  string `json:"folderId"`
	SessionID string `json:"sessionId"`
}

func (d VarsScopeDTO) scope() vars.Scope {
	return vars.Scope{FolderID: d.FolderID, SessionID: d.SessionID}
}

// varsChain adapts the session store's folder chain to vars.ChainFunc
// (root -> leaf folder IDs).
func (a *App) varsChain(folderID string) []string {
	ch := a.folderChain(folderID)
	ids := make([]string, len(ch))
	for i, f := range ch {
		ids[i] = f.ID
	}
	return ids
}

// VarsList returns the resolved variables visible at a scope.
func (a *App) VarsList(scope VarsScopeDTO) map[string]string {
	return a.varStore.List(scope.scope())
}

// VarsPut sets a variable at the most specific level of scope.
func (a *App) VarsPut(scope VarsScopeDTO, key, value string) error {
	return a.varStore.Put(scope.scope(), key, value)
}

// VarsDelete removes a variable at the scope's level.
func (a *App) VarsDelete(scope VarsScopeDTO, key string) error {
	return a.varStore.Delete(scope.scope(), key)
}

// resolvedVars returns the fully resolved variables for a session (its folder
// chain overlaid by the session scope).
func (a *App) resolvedVars(sessionID string) (map[string]string, error) {
	s, _, err := a.st.Resolve(sessionID)
	if err != nil {
		return nil, err
	}
	return a.varStore.List(vars.Scope{FolderID: s.FolderID, SessionID: sessionID}), nil
}

// TemplateUnresolved returns the template variables that cannot be resolved for
// a session — the set to prompt for before pasting.
func (a *App) TemplateUnresolved(sessionID, body string) ([]string, error) {
	vs, err := a.resolvedVars(sessionID)
	if err != nil {
		return nil, err
	}
	return snippets.Unresolved(body, vs), nil
}

// RenderTemplate renders body with a session's resolved vars overlaid by extra
// (e.g. prompt answers). A preview / dry-run helper.
func (a *App) RenderTemplate(sessionID, body string, extra map[string]string) (string, error) {
	return a.renderFor(sessionID, body, extra)
}

func (a *App) renderFor(sessionID, body string, extra map[string]string) (string, error) {
	vs, err := a.resolvedVars(sessionID)
	if err != nil {
		return "", err
	}
	for k, v := range extra {
		vs[k] = v
	}
	return snippets.Render(body, vs)
}

// SendToTerminal writes already-rendered text to a terminal with a paste mode:
// bracketed-paste, per-line typing with an inter-line delay, or a single write.
// Newlines become carriage returns so each line executes.
func (a *App) SendToTerminal(termID, text string, lineDelayMs int, bracketed bool) error {
	return a.sendText(termID, text, lineDelayMs, bracketed)
}

// SendTemplate renders body against the terminal's session vars (overlaid by
// extra) and sends it with the given paste mode.
func (a *App) SendTemplate(termID, body string, extra map[string]string, lineDelayMs int, bracketed bool) error {
	a.tmu.Lock()
	t, ok := a.terms[termID]
	a.tmu.Unlock()
	if !ok {
		return fmt.Errorf("app: terminal not open")
	}
	text, err := a.renderFor(t.sessionID, body, extra)
	if err != nil {
		return err
	}
	return a.sendText(termID, text, lineDelayMs, bracketed)
}

func (a *App) sendText(termID, text string, lineDelayMs int, bracketed bool) error {
	a.tmu.Lock()
	t, ok := a.terms[termID]
	a.tmu.Unlock()
	if !ok {
		return fmt.Errorf("app: terminal not open")
	}
	w := t.session.Stdin()
	if bracketed {
		_, err := w.Write([]byte(snippets.BracketedWrap(text)))
		return err
	}
	lines := sendLines(text)
	if len(lines) == 0 {
		return nil
	}
	if lineDelayMs <= 0 {
		_, err := w.Write([]byte(strings.Join(lines, "")))
		return err
	}
	if lineDelayMs > 5000 {
		lineDelayMs = 5000 // sanity cap
	}
	for i, ln := range lines {
		if i > 0 {
			time.Sleep(time.Duration(lineDelayMs) * time.Millisecond)
			a.tmu.Lock()
			_, still := a.terms[termID]
			a.tmu.Unlock()
			if !still {
				return fmt.Errorf("app: terminal closed during paste")
			}
		}
		if _, err := w.Write([]byte(ln)); err != nil {
			return err
		}
	}
	return nil
}

// sendLines splits text into terminal lines each terminated by CR, dropping a
// single trailing empty line so "cmd\n" sends exactly one Enter.
func sendLines(text string) []string {
	if text == "" {
		return nil
	}
	parts := strings.Split(text, "\n")
	if n := len(parts); n > 1 && parts[n-1] == "" {
		parts = parts[:n-1]
	}
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = p + "\r"
	}
	return out
}
