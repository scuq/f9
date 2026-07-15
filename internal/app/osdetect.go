package app

import (
	"github.com/scuq/f9/internal/osdetect"
	"github.com/scuq/f9/internal/store"
)

// detectorRelay reports whether the session's effective chain contains a
// shell-hop, in which case detection must ignore hop banner evidence.
func (a *App) detectorRelay(sessionID string) bool {
	_, eff, err := a.st.Resolve(sessionID)
	if err != nil {
		return false
	}
	for _, h := range eff.JumpChain {
		if h.Mode == "shell-hop" {
			return true
		}
	}
	return false
}

// SessionRedetectOS clears the detected OS (session meta AND the host's
// shadow-cache hint) and, when the session is connected, immediately starts a
// fresh detection round.
func (a *App) SessionRedetectOS(sessionID string) error {
	m, err := a.st.Meta(sessionID)
	if err != nil {
		return err
	}
	m.SessionID = sessionID
	m.DetectedOS = ""
	m.OSConfidence = 0
	m.OSPinned = false
	if err := a.st.PutMeta(m); err != nil {
		return err
	}
	if s, _, rerr := a.st.Resolve(sessionID); rerr == nil && s.Host != "" {
		_ = a.st.DeleteOSHint(s.Host)
	}
	if client, ok := a.mgr.Client(sessionID); ok {
		relay := a.detectorRelay(sessionID)
		sv := client.ServerVersion()
		if relay {
			sv = ""
		}
		a.detMu.Lock()
		delete(a.dets, sessionID)
		a.detMu.Unlock()
		a.ensureDetector(sessionID, sv, relay)
	}
	return nil
}

// ensureDetector starts passive OS detection for a session with no settled OS
// yet, seeding it with the SSH server version string. Safe to call for every
// opened terminal; only the first call per session creates a detector.
func (a *App) ensureDetector(sessionID, serverVersion string, relay bool) {
	if m, err := a.st.Meta(sessionID); err == nil && (m.DetectedOS != "" || m.OSPinned) {
		return
	}
	a.detMu.Lock()
	defer a.detMu.Unlock()
	if a.dets == nil {
		a.dets = map[string]osdetect.Detector{}
	}
	if _, ok := a.dets[sessionID]; ok {
		return
	}
	det := osdetect.New()
	if relay {
		det = osdetect.NewRelay()
	}
	if serverVersion != "" {
		det.ObserveServerVersion(serverVersion)
	}
	a.dets[sessionID] = det
}

// observeOS feeds terminal output into the session's detector and, once the
// guess crosses the confidence threshold, persists it: session meta (skipped
// when the OS is pinned) plus the host-keyed OS hint cache, so the knowledge
// survives import resync. Detection stops for the session after settling.
func (a *App) observeOS(sessionID string, p []byte) {
	a.detMu.Lock()
	det, ok := a.dets[sessionID]
	a.detMu.Unlock()
	if !ok {
		return
	}
	det.ObserveOutput(p)
	g := det.Guess()
	if g.Family == osdetect.FamilyUnknown || g.Confidence < osdetect.DefaultThreshold {
		return
	}
	a.detMu.Lock()
	delete(a.dets, sessionID)
	a.detMu.Unlock()

	m, err := a.st.Meta(sessionID)
	if err != nil || m.OSPinned {
		return
	}
	m.SessionID = sessionID
	m.DetectedOS = string(g.Family)
	m.OSConfidence = g.Confidence
	if a.st.PutMeta(m) != nil {
		return
	}
	if s, _, err := a.st.Resolve(sessionID); err == nil && s.Host != "" {
		_ = a.st.PutOSHint(store.OSHint{Host: s.Host, OS: string(g.Family), Confidence: g.Confidence})
	}
	a.emitEvent("f9:osdetected", map[string]interface{}{"sessionId": sessionID, "os": string(g.Family)})
}

// dropDetectorIfIdle removes the session's detector once its last terminal
// closed without the guess ever settling.
func (a *App) dropDetectorIfIdle(sessionID string) {
	a.tmu.Lock()
	remaining := false
	for _, t := range a.terms {
		if t.sessionID == sessionID {
			remaining = true
			break
		}
	}
	a.tmu.Unlock()
	if !remaining {
		a.detMu.Lock()
		delete(a.dets, sessionID)
		a.detMu.Unlock()
	}
}
