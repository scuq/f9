package app

import "github.com/scuq/f9/internal/store"

// PinSession marks a session as pinned (persisted in SessionMeta). Pinned
// sessions render as always-visible tabs in the strip.
func (a *App) PinSession(sessionID string) error {
	m, _ := a.st.Meta(sessionID)
	m.SessionID = sessionID
	m.Pinned = true
	return a.st.PutMeta(m)
}

// UnpinSession clears the pinned flag.
func (a *App) UnpinSession(sessionID string) error {
	m, _ := a.st.Meta(sessionID)
	m.SessionID = sessionID
	m.Pinned = false
	return a.st.PutMeta(m)
}

// PinnedSessions returns all pinned sessions (fresh from disk).
func (a *App) PinnedSessions() ([]SessionNode, error) {
	if err := a.st.LoadAll(); err != nil {
		return nil, err
	}
	var out []SessionNode
	for _, s := range a.st.Sessions() {
		if m, err := a.st.Meta(s.ID); err == nil && m.Pinned {
			out = append(out, a.sessionNode(s))
		}
	}
	return out, nil
}

var _ = store.SessionMeta{} // keep store imported if sessionNode signature changes
