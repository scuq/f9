package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/scuq/f9/internal/connmgr"
	"github.com/scuq/f9/internal/sshx"
	"github.com/scuq/f9/internal/store"
)

// ---- prompt DTOs (cross the Wails boundary) ----

type PromptRequest struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"` // password | passphrase | hostkey | kbi
	User        string `json:"user"`
	Host        string `json:"host"`
	KeyPath     string `json:"keyPath"`
	Fingerprint string `json:"fingerprint"`
	Prompt      string `json:"prompt"`
	Echo        bool   `json:"echo"`
}

type PromptReply struct {
	Value     string `json:"value"`
	UseForAll bool   `json:"useForAll"`
	Accept    bool   `json:"accept"`
	Cancel    bool   `json:"cancel"`
}

var errPromptCancelled = errors.New("app: prompt cancelled")

// promptRequester is satisfied by *App; abstracted so promptBridge is testable.
type promptRequester interface {
	requestPrompt(req PromptRequest) (PromptReply, error)
}

// promptBridge is the batch-scoped sshx.Prompter. Password prompts serialize
// on mu so the "use for all" answer governs the rest of the batch; passphrases
// share by key path; host-key confirms always prompt (per-host keys).
type promptBridge struct {
	req         promptRequester
	mu          sync.Mutex
	shared      *string
	passphrases map[string]string
}

func newPromptBridge(r promptRequester) *promptBridge {
	return &promptBridge{req: r, passphrases: map[string]string{}}
}

func (b *promptBridge) Password(user, host string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.shared != nil {
		return *b.shared, nil
	}
	r, err := b.req.requestPrompt(PromptRequest{
		Kind: "password", User: user, Host: host,
		Prompt: fmt.Sprintf("Password for %s@%s", user, host),
	})
	if err != nil {
		return "", err
	}
	if r.UseForAll {
		v := r.Value
		b.shared = &v
	}
	return r.Value, nil
}

func (b *promptBridge) Passphrase(keyPath string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if v, ok := b.passphrases[keyPath]; ok {
		return v, nil
	}
	r, err := b.req.requestPrompt(PromptRequest{
		Kind: "passphrase", KeyPath: keyPath,
		Prompt: "Passphrase for " + keyPath,
	})
	if err != nil {
		return "", err
	}
	b.passphrases[keyPath] = r.Value
	return r.Value, nil
}

func (b *promptBridge) KeyboardInteractive(name, instruction string, questions []string, echos []bool) ([]string, error) {
	ans := make([]string, len(questions))
	for i, q := range questions {
		echo := false
		if i < len(echos) {
			echo = echos[i]
		}
		r, err := b.req.requestPrompt(PromptRequest{Kind: "kbi", Prompt: q, Echo: echo})
		if err != nil {
			return nil, err
		}
		ans[i] = r.Value
	}
	return ans, nil
}

func (b *promptBridge) ConfirmHostKey(host, fingerprint string) (bool, error) {
	r, err := b.req.requestPrompt(PromptRequest{
		Kind: "hostkey", Host: host, Fingerprint: fingerprint,
		Prompt: "Unknown host " + host,
	})
	if err != nil {
		return false, err
	}
	return r.Accept, nil
}

// ---- prompt routing on *App ----

func (a *App) nextReqID() string {
	a.pmu.Lock()
	a.reqSeq++
	n := a.reqSeq
	a.pmu.Unlock()
	return fmt.Sprintf("p%d-%d", time.Now().UnixNano(), n)
}

func (a *App) requestPrompt(req PromptRequest) (PromptReply, error) {
	req.ID = a.nextReqID()
	ch := make(chan PromptReply, 1)
	a.pmu.Lock()
	a.prompts[req.ID] = ch
	a.pmu.Unlock()
	defer func() {
		a.pmu.Lock()
		delete(a.prompts, req.ID)
		a.pmu.Unlock()
	}()

	a.emitEvent("f9:prompt", req)

	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case r := <-ch:
		if r.Cancel {
			return r, errPromptCancelled
		}
		return r, nil
	case <-ctx.Done():
		return PromptReply{}, ctx.Err()
	}
}

// ResolvePrompt is called by the frontend to answer a pending prompt.
func (a *App) ResolvePrompt(id string, reply PromptReply) {
	a.pmu.Lock()
	ch, ok := a.prompts[id]
	a.pmu.Unlock()
	if ok {
		select {
		case ch <- reply:
		default:
		}
	}
}

func (a *App) emitConns() { a.emitEvent("f9:conns", nil) }

// ---- connect bindings ----

// ConnectSessions dials the given session IDs as one batch (shared prompter).
// resolveTargetUser returns the username used for the final target. The
// session's own user takes precedence; a shell-hop's user override on the last
// hop is only a fallback (e.g. a default carried by an inherited folder jump
// chain) applied when the session has no user of its own. This lets a per-device
// user set by an import map script win over a folder-wide override.
func resolveTargetUser(sessionUser string, chain []store.JumpHop) string {
	if sessionUser != "" {
		return sessionUser
	}
	if n := len(chain); n > 0 {
		last := chain[n-1]
		if last.Mode == "shell-hop" && last.UserOverride != "" {
			return last.UserOverride
		}
	}
	return sessionUser
}

func (a *App) ConnectSessions(ids []string) error {
	gs := a.Settings()
	targets := make([]connmgr.Target, 0, len(ids))
	for _, id := range ids {
		s, eff, err := a.st.Resolve(id)
		if err != nil {
			return err
		}
		keyFiles := gs.KeyFiles
		if eff.KeyFile != nil && *eff.KeyFile != "" {
			keyFiles = []string{*eff.KeyFile}
		}
		noAgent := gs.DisableAgent
		if eff.UseAgent != nil {
			noAgent = !*eff.UseAgent
		}
		t := connmgr.Target{
			SessionID: s.ID, Name: s.Name, Host: s.Host, Port: s.Port, User: s.User,
			Keepalive:    30 * time.Second,
			KeyFiles:     keyFiles,
			NoAgent:      noAgent,
			AgentSockets: gs.AgentSockets,
		}
		if eff.KeepaliveInterval != nil {
			t.Keepalive = *eff.KeepaliveInterval
		}
		if eff.SocksPort != nil {
			t.SocksPort = *eff.SocksPort
		}
		if eff.SocksOnly != nil {
			t.SocksOnly = *eff.SocksOnly
		}
		for _, j := range eff.JumpChain {
			t.JumpChain = append(t.JumpChain, sshx.Hop{Host: j.Host, Port: j.Port, User: j.User, Mode: j.Mode})
		}
		t.User = resolveTargetUser(s.User, eff.JumpChain)
		targets = append(targets, t)
	}
	if len(targets) == 0 {
		return nil
	}
	a.mgr.ConnectBatch(context.Background(), targets, newPromptBridge(a))
	return nil
}

// ConnectFolder dials every session under a folder (recursively).
func (a *App) ConnectFolder(folderID string) error {
	if err := a.st.LoadAll(); err != nil {
		return err
	}
	want := a.descendantFolders(folderID)
	var ids []string
	for _, s := range a.st.Sessions() {
		if want[s.FolderID] {
			ids = append(ids, s.ID)
		}
	}
	return a.ConnectSessions(ids)
}

func (a *App) descendantFolders(root string) map[string]bool {
	children := map[string][]string{}
	for _, f := range a.st.Folders() {
		children[f.ParentID] = append(children[f.ParentID], f.ID)
	}
	out := map[string]bool{}
	var walk func(string)
	walk = func(id string) {
		out[id] = true
		for _, c := range children[id] {
			walk(c)
		}
	}
	walk(root)
	return out
}

func (a *App) ActiveConnections() []connmgr.Conn { return a.mgr.Active() }
func (a *App) Disconnect(id string)              { a.mgr.Disconnect(id) }
func (a *App) DisconnectAll()                    { a.mgr.DisconnectAll() }
