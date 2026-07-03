package app

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/scuq/f9/internal/sshx"
)

// terminal wraps one interactive SSH channel opened on a connected client.
// Data is base64-encoded and streamed to the frontend over a per-terminal
// Wails event so non-UTF-8 escape sequences survive the JSON boundary.
type terminal struct {
	session sshx.Session
}

// OpenTerminal opens a shell channel on the (already connected) session and
// begins streaming output as "f9:term:<sessionID>" events. Idempotent: a second
// call for an open terminal is a no-op (focus is handled frontend-side).
func (a *App) OpenTerminal(sessionID string, cols, rows int) error {
	a.tmu.Lock()
	if _, ok := a.terms[sessionID]; ok {
		a.tmu.Unlock()
		return nil
	}
	a.tmu.Unlock()

	client, ok := a.mgr.Client(sessionID)
	if !ok {
		return fmt.Errorf("app: session not connected")
	}
	sess, err := client.NewSession(context.Background(), "xterm-256color", cols, rows)
	if err != nil {
		return err
	}

	a.tmu.Lock()
	a.terms[sessionID] = &terminal{session: sess}
	a.tmu.Unlock()

	dataEvent := "f9:term:" + sessionID
	sess.OnData(func(p []byte) {
		a.emitEvent(dataEvent, base64.StdEncoding.EncodeToString(p))
	})
	go func() {
		_ = sess.Wait()
		a.tmu.Lock()
		delete(a.terms, sessionID)
		a.tmu.Unlock()
		a.emitEvent("f9:termclose:"+sessionID, nil)
	}()
	return nil
}

// TermInput forwards keystrokes to the session's stdin.
func (a *App) TermInput(sessionID, data string) {
	a.tmu.Lock()
	t, ok := a.terms[sessionID]
	a.tmu.Unlock()
	if ok {
		_, _ = t.session.Stdin().Write([]byte(data))
	}
}

// TermResize propagates a terminal resize to the remote PTY.
func (a *App) TermResize(sessionID string, cols, rows int) {
	a.tmu.Lock()
	t, ok := a.terms[sessionID]
	a.tmu.Unlock()
	if ok {
		_ = t.session.Resize(cols, rows)
	}
}

// CloseTerminal closes the shell channel (the connmgr client stays connected).
func (a *App) CloseTerminal(sessionID string) {
	a.tmu.Lock()
	t, ok := a.terms[sessionID]
	delete(a.terms, sessionID)
	a.tmu.Unlock()
	if ok {
		_ = t.session.Close()
	}
}
