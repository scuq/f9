// Package app is the thin binding layer between the Wails frontend and the
// engine packages (store, filter, sshx, scrollback, osdetect). It translates
// UI calls only — no business logic lives here (phase-plan 01). It imports no
// Wails packages so it compiles on every GOOS/GOARCH without cgo.
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/scuq/f9/internal/buttonbar"
	"github.com/scuq/f9/internal/connmgr"
	"github.com/scuq/f9/internal/cred"
	"github.com/scuq/f9/internal/filter"
	"github.com/scuq/f9/internal/luamap"
	"github.com/scuq/f9/internal/multisend"
	"github.com/scuq/f9/internal/osdetect"
	"github.com/scuq/f9/internal/snippets"
	"github.com/scuq/f9/internal/sshx"
	"github.com/scuq/f9/internal/store"
	"github.com/scuq/f9/internal/theme"
	"github.com/scuq/f9/internal/vars"
)

// Version is the GUI-facing version string, injected at build time via
// -ldflags "-X github.com/scuq/f9/internal/app.Version=<version>" (see the
// Makefile LDFLAGS + the VERSION file). Defaults to "dev" for un-stamped builds.
var Version = "dev"

// ---- tree ----

type SessionNode struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	User       string `json:"user"`
	Proto      string `json:"proto"`
	DetectedOS string `json:"detectedOs"`
	OSPinned   bool   `json:"osPinned"`
	Pinned     bool   `json:"pinned"`
	Generated  bool   `json:"generated"`
}

type FolderNode struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Path      string        `json:"path"`
	HasSource bool          `json:"hasSource"`
	Folders   []*FolderNode `json:"folders"`
	Sessions  []SessionNode `json:"sessions"`
}

// ---- filter ----

type FilterHit struct {
	SessionNode
	Path  string `json:"path"`
	Score int    `json:"score"`
}

// ---- detail / provenance ----

// OptionField is one inheritable option as the inheritance view renders it:
// the session's own value (empty = inherited), the resolved effective value,
// and where it came from ("session", "folder: <path>", "defaults", "unset").
type OptionField struct {
	Value     string `json:"value"`
	Effective string `json:"effective"`
	Source    string `json:"source"`
}

type JumpHopDTO struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	User         string `json:"user"`
	Mode         string `json:"mode"`
	UserOverride string `json:"userOverride"`
}

type SessionDetail struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	FolderID   string                 `json:"folderId"`
	FolderPath string                 `json:"folderPath"`
	Host       string                 `json:"host"`
	Port       int                    `json:"port"`
	User       string                 `json:"user"`
	Proto      string                 `json:"proto"`
	Options    map[string]OptionField `json:"options"`
	JumpChain  []JumpHopDTO           `json:"jumpChain"`
	JumpSource string                 `json:"jumpSource"`
	OnwardUser string                 `json:"onwardUser"` // effective target login (session user, else shell-hop fallback)
}

// ---- save inputs ----

// SessionInput: Options carries all option keys; an empty value means
// "inherit" (clears any session-level override).
type SessionInput struct {
	ID       string            `json:"id"`
	FolderID string            `json:"folderId"`
	Name     string            `json:"name"`
	Host     string            `json:"host"`
	Port     int               `json:"port"`
	User     string            `json:"user"`
	Proto    string            `json:"proto"`
	Options  map[string]string `json:"options"`
}

type FolderInput struct {
	ID       string `json:"id"`
	ParentID string `json:"parentId"`
	Name     string `json:"name"`
}

type App struct {
	ctx      context.Context
	st       *store.YAMLStore
	varStore *vars.YAMLStore
	bars     *buttonbar.YAMLStore
	snips    *snippets.Store
	creds    *cred.Store
	maps     *luamap.Library
	mgr      *connmgr.Manager

	pmu     sync.Mutex
	prompts map[string]chan PromptReply
	reqSeq  int64

	tmu     sync.Mutex
	terms   map[string]*terminal
	tunings map[osdetect.Family]osdetect.Tuning

	detMu sync.Mutex
	dets  map[string]osdetect.Detector

	msMu     sync.RWMutex
	msJob    *multisend.Job
	msCancel chan struct{}

	refreshMu      sync.Mutex
	refreshCancels map[string]context.CancelFunc

	themes    map[string]*theme.Theme
	themeName string
	themeMu   sync.RWMutex

	// onEmit is a test hook used only by the non-gui emitEvent stub.
	onEmit func(event string, data interface{})
}

func New() (*App, error) {
	root, err := storeRoot()
	if err != nil {
		return nil, err
	}
	st, err := store.Open(root)
	if err != nil {
		return nil, err
	}
	a := &App{
		st:      st,
		prompts: map[string]chan PromptReply{},
		terms:   map[string]*terminal{},
		tunings: loadTunings(),
		themes:  theme.LoadAll(),
	}
	a.themeName = initialThemeName(a.themes)
	a.mgr = connmgr.New(64, sshx.Dial, a.emitConns)
	vstore, err := vars.Open(filepath.Join(root, ".vars"), a.varsChain)
	if err != nil {
		return nil, err
	}
	a.varStore = vstore
	bstore, err := buttonbar.Open(filepath.Join(root, ".bars"), a.varsChain)
	if err != nil {
		return nil, err
	}
	a.bars = bstore
	sstore, err := snippets.Open(filepath.Join(root, ".snippets"))
	if err != nil {
		return nil, err
	}
	a.snips = sstore
	cstore, err := cred.Open(filepath.Join(root, ".secrets.yaml"))
	if err != nil {
		return nil, err
	}
	a.creds = cstore
	mstore, err := luamap.OpenLibrary(filepath.Join(root, ".mapscripts.yaml"))
	if err != nil {
		return nil, err
	}
	a.maps = mstore
	return a, nil
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.startThemeWatcher()
}

func (a *App) GetVersion() string { return Version }

// Tree reloads the store from disk and returns the full folder/session tree.
// Reload-on-call keeps hand-edited YAML visible without a watcher.
func (a *App) Tree() (*FolderNode, error) {
	if err := a.st.LoadAll(); err != nil {
		return nil, err
	}
	nodes := map[string]*FolderNode{}
	var root *FolderNode
	for _, f := range a.st.Folders() { // sorted by (ParentID, Name)
		nodes[f.ID] = &FolderNode{ID: f.ID, Name: f.Name, Path: a.st.FolderPath(f.ID), HasSource: f.Source != nil}
	}
	for _, f := range a.st.Folders() {
		n := nodes[f.ID]
		if f.ParentID == "" {
			root = n
			continue
		}
		if p, ok := nodes[f.ParentID]; ok {
			p.Folders = append(p.Folders, n)
		}
	}
	if root == nil {
		return nil, fmt.Errorf("app: store has no root folder")
	}
	for _, s := range a.st.Sessions() { // sorted by (FolderID, Name)
		n, ok := nodes[s.FolderID]
		if !ok {
			continue
		}
		n.Sessions = append(n.Sessions, a.sessionNode(s))
	}
	return root, nil
}

// Filter ranks all sessions against query (internal/filter; <5ms/10k budget).
// Called per keystroke — uses the in-memory index, no disk reload.
func (a *App) Filter(query string) ([]FilterHit, error) {
	sessions := a.st.Sessions()
	items := make([]filter.Item, len(sessions))
	byID := make(map[string]store.Session, len(sessions))
	for i, s := range sessions {
		items[i] = filter.Item{
			ID:   s.ID,
			Name: s.Name,
			Path: a.st.FolderPath(s.FolderID),
			Host: s.Host,
			Tags: s.Tags,
		}
		byID[s.ID] = s
	}
	hits := filter.Rank(query, items)
	out := make([]FilterHit, 0, len(hits))
	for _, h := range hits {
		s := byID[h.ID]
		out = append(out, FilterHit{
			SessionNode: a.sessionNode(s),
			Path:        h.Path,
			Score:       h.Score,
		})
	}
	return out, nil
}

// SessionDetail returns one session with its full inheritance view.
func (a *App) SessionDetail(id string) (*SessionDetail, error) {
	s, eff, err := a.st.Resolve(id)
	if err != nil {
		return nil, err
	}
	chain := a.folderChain(s.FolderID) // root .. leaf
	d := &SessionDetail{
		ID:         s.ID,
		Name:       s.Name,
		FolderID:   s.FolderID,
		FolderPath: a.st.FolderPath(s.FolderID),
		Host:       s.Host,
		Port:       s.Port,
		User:       s.User,
		Proto:      s.Proto,
		Options:    a.optionFields(s, eff, chain),
	}
	for _, j := range eff.JumpChain {
		d.JumpChain = append(d.JumpChain, JumpHopDTO{
			Host: j.Host, Port: j.Port, User: j.User,
			Mode: j.Mode, UserOverride: j.UserOverride,
		})
	}
	d.JumpSource = a.sourceOf(chain,
		func(o store.SessionOptions) bool { return o.JumpChain != nil },
		s.Options.JumpChain != nil)
	ou, ochain, _ := resolveAltRefs(a.Settings().AltUsers, s.User, eff.JumpChain)
	d.OnwardUser = resolveTargetUser(ou, ochain)
	return d, nil
}

// SaveSession creates (empty ID) or updates a session; returns the ID.
// On update the folder is preserved — moving sessions between folders comes
// with the tree drag/drop work (01c+), like the jump-chain editor.
func (a *App) SaveSession(in SessionInput) (string, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Host = strings.TrimSpace(in.Host)
	if in.Name == "" || in.Host == "" {
		return "", fmt.Errorf("app: name and host are required")
	}
	var s store.Session
	if in.ID != "" {
		existing, _, err := a.st.Resolve(in.ID)
		if err != nil {
			return "", err
		}
		s = existing // keeps ID, FolderID, Tags, Revision handling
	} else {
		folderID := in.FolderID
		if folderID == "" {
			folderID = a.st.RootID()
		}
		s = store.Session{FolderID: folderID}
	}
	s.Name = in.Name
	s.Host = in.Host
	s.Port = in.Port
	s.User = strings.TrimSpace(in.User)
	s.Proto = strings.TrimSpace(in.Proto)
	if s.Proto == "" {
		s.Proto = "ssh"
	}
	jump := s.Options.JumpChain // preserved: edited elsewhere
	opts, err := parseOptions(in.Options)
	if err != nil {
		return "", err
	}
	opts.JumpChain = jump
	s.Options = opts

	if err := a.st.Put(s); err != nil {
		return "", err
	}
	return a.sessionIDByName(s.FolderID, s.Name)
}

// SessionSetJumpChain replaces a session's jump chain (hops applied in order
// before reaching the target). An empty list clears it (overriding inheritance).
func toJumpChain(hops []JumpHopDTO) ([]store.JumpHop, error) {
	chain := make([]store.JumpHop, 0, len(hops))
	for _, h := range hops {
		host := strings.TrimSpace(h.Host)
		if host == "" {
			return nil, fmt.Errorf("app: jump hop host required")
		}
		mode := strings.TrimSpace(h.Mode)
		if mode == "" {
			mode = "proxyjump"
		}
		if mode != "proxyjump" && mode != "shell-hop" {
			return nil, fmt.Errorf("app: jump mode %q: want proxyjump|shell-hop", mode)
		}
		port := h.Port
		if port == 0 {
			port = 22
		}
		chain = append(chain, store.JumpHop{
			Host: host, Port: port, User: strings.TrimSpace(h.User),
			Mode: mode, UserOverride: strings.TrimSpace(h.UserOverride),
		})
	}
	return chain, nil
}

func jumpChainDTO(chain []store.JumpHop) []JumpHopDTO {
	out := make([]JumpHopDTO, 0, len(chain))
	for _, h := range chain {
		out = append(out, JumpHopDTO{Host: h.Host, Port: h.Port, User: h.User, Mode: h.Mode, UserOverride: h.UserOverride})
	}
	return out
}

func (a *App) SessionSetJumpChain(sessionID string, hops []JumpHopDTO) error {
	s, _, err := a.st.Resolve(sessionID)
	if err != nil {
		return err
	}
	chain, err := toJumpChain(hops)
	if err != nil {
		return err
	}
	s.Options.JumpChain = chain
	return a.st.Put(s)
}

// FolderJumpChain returns a folder's jump chain (applied to sessions under it).
func (a *App) FolderJumpChain(folderID string) []JumpHopDTO {
	for _, f := range a.st.Folders() {
		if f.ID == folderID {
			return jumpChainDTO(f.Options.JumpChain)
		}
	}
	return nil
}

// FolderSetJumpChain sets a folder's jump chain; every session under the folder
// inherits it through the option overlay unless it overrides.
func (a *App) FolderSetJumpChain(folderID string, hops []JumpHopDTO) error {
	var f store.Folder
	found := false
	for _, x := range a.st.Folders() {
		if x.ID == folderID {
			f = x
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("app: folder %s not found", folderID)
	}
	chain, err := toJumpChain(hops)
	if err != nil {
		return err
	}
	f.Options.JumpChain = chain
	return a.st.PutFolder(f)
}

// SessionDuplicate copies a session (including its options) into the same
// folder under a unique name, as a hand-made (non-generated) session — so a
// duplicate of an imported session is a normal, refresh-safe local copy.
func (a *App) SessionDuplicate(sessionID string) (string, error) {
	src, _, err := a.st.Resolve(sessionID)
	if err != nil {
		return "", err
	}
	dup := src
	dup.ID = ""
	dup.Source = "" // a hand-made copy, not managed by an import source
	dup.ExternalID = ""
	dup.Revision = 0
	base := src.Name + " copy"
	name := base
	for i := 2; ; i++ {
		if _, exists := a.st.SessionByName(src.FolderID, name); !exists {
			break
		}
		name = fmt.Sprintf("%s %d", base, i)
	}
	dup.Name = name
	if err := a.st.Put(dup); err != nil {
		return "", err
	}
	return a.sessionIDByName(dup.FolderID, dup.Name)
}

// SaveFolder creates (empty ID) or renames a folder; returns the ID.
func (a *App) SaveFolder(in FolderInput) (string, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return "", fmt.Errorf("app: folder name is required")
	}
	var f store.Folder
	if in.ID != "" {
		for _, x := range a.st.Folders() {
			if x.ID == in.ID {
				f = x
				break
			}
		}
		if f.ID == "" {
			return "", fmt.Errorf("app: folder %s not found", in.ID)
		}
	} else {
		parent := in.ParentID
		if parent == "" {
			parent = a.st.RootID()
		}
		f = store.Folder{ParentID: parent}
	}
	f.Name = in.Name
	if err := a.st.PutFolder(f); err != nil {
		return "", err
	}
	nf, ok := a.st.FolderByName(f.ParentID, f.Name)
	if !ok {
		return "", fmt.Errorf("app: folder %q not found after save", f.Name)
	}
	return nf.ID, nil
}

func (a *App) DeleteSession(id string) error { return a.st.Delete(id) }
func (a *App) DeleteFolder(id string) error  { return a.st.Delete(id) }

// ---- helpers ----

func (a *App) sessionNode(s store.Session) SessionNode {
	sn := SessionNode{
		ID: s.ID, Name: s.Name, Host: s.Host, Port: s.Port,
		User: s.User, Proto: s.Proto,
	}
	sn.Generated = s.Source != ""
	if m, err := a.st.Meta(s.ID); err == nil {
		sn.DetectedOS = m.DetectedOS
		sn.OSPinned = m.OSPinned
		sn.Pinned = m.Pinned
	}
	return sn
}

// folderChain returns root..leaf for a folder ID.
func (a *App) folderChain(folderID string) []store.Folder {
	byID := map[string]store.Folder{}
	for _, f := range a.st.Folders() {
		byID[f.ID] = f
	}
	var chain []store.Folder
	for id := folderID; id != ""; {
		f, ok := byID[id]
		if !ok {
			break
		}
		chain = append(chain, f)
		id = f.ParentID
	}
	// reverse to root..leaf
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

// sourceOf labels where an option's effective value comes from. chain is
// root..leaf; nearest provider wins, mirroring the store's overlay order.
func (a *App) sourceOf(chain []store.Folder, has func(store.SessionOptions) bool, ownSet bool) string {
	if ownSet {
		return "session"
	}
	for i := len(chain) - 1; i >= 0; i-- {
		if has(chain[i].Options) {
			if chain[i].ParentID == "" {
				return "defaults"
			}
			return "folder: " + a.st.FolderPath(chain[i].ID)
		}
	}
	return "unset"
}

// optionFields builds the inheritance view. NOTE: covers every field of
// store.SessionOptions except JumpChain (rendered separately); the reflect
// guard in app_test.go fails the build's tests when SessionOptions grows.
func (a *App) optionFields(s store.Session, eff store.SessionOptions, chain []store.Folder) map[string]OptionField {
	out := map[string]OptionField{}

	out["termType"] = OptionField{
		Value:     strPtr(s.Options.TermType),
		Effective: strPtr(eff.TermType),
		Source: a.sourceOf(chain,
			func(o store.SessionOptions) bool { return o.TermType != nil },
			s.Options.TermType != nil),
	}
	out["keepaliveInterval"] = OptionField{
		Value:     durPtr(s.Options.KeepaliveInterval),
		Effective: durPtr(eff.KeepaliveInterval),
		Source: a.sourceOf(chain,
			func(o store.SessionOptions) bool { return o.KeepaliveInterval != nil },
			s.Options.KeepaliveInterval != nil),
	}
	out["reconnect"] = OptionField{
		Value:     strPtr(s.Options.Reconnect),
		Effective: strPtr(eff.Reconnect),
		Source: a.sourceOf(chain,
			func(o store.SessionOptions) bool { return o.Reconnect != nil },
			s.Options.Reconnect != nil),
	}
	out["theme"] = OptionField{
		Value:     strPtr(s.Options.ThemeRef),
		Effective: strPtr(eff.ThemeRef),
		Source: a.sourceOf(chain,
			func(o store.SessionOptions) bool { return o.ThemeRef != nil },
			s.Options.ThemeRef != nil),
	}
	out["scrollbackLines"] = OptionField{
		Value:     intPtr(s.Options.ScrollbackLines),
		Effective: intPtr(eff.ScrollbackLines),
		Source: a.sourceOf(chain,
			func(o store.SessionOptions) bool { return o.ScrollbackLines != nil },
			s.Options.ScrollbackLines != nil),
	}
	out["auditScope"] = OptionField{
		Value:     strPtr(s.Options.AuditScope),
		Effective: strPtr(eff.AuditScope),
		Source: a.sourceOf(chain,
			func(o store.SessionOptions) bool { return o.AuditScope != nil },
			s.Options.AuditScope != nil),
	}
	out["keyFile"] = OptionField{
		Value:     strPtr(s.Options.KeyFile),
		Effective: strPtr(eff.KeyFile),
		Source: a.sourceOf(chain,
			func(o store.SessionOptions) bool { return o.KeyFile != nil },
			s.Options.KeyFile != nil),
	}
	out["useAgent"] = OptionField{
		Value:     boolPtr(s.Options.UseAgent),
		Effective: boolPtr(eff.UseAgent),
		Source: a.sourceOf(chain,
			func(o store.SessionOptions) bool { return o.UseAgent != nil },
			s.Options.UseAgent != nil),
	}
	out["socksPort"] = OptionField{
		Value:     intPtr(s.Options.SocksPort),
		Effective: intPtr(eff.SocksPort),
		Source: a.sourceOf(chain,
			func(o store.SessionOptions) bool { return o.SocksPort != nil },
			s.Options.SocksPort != nil),
	}
	out["socksOnly"] = OptionField{
		Value:     boolPtr(s.Options.SocksOnly),
		Effective: boolPtr(eff.SocksOnly),
		Source: a.sourceOf(chain,
			func(o store.SessionOptions) bool { return o.SocksOnly != nil },
			s.Options.SocksOnly != nil),
	}
	return out
}

// parseOptions converts the UI's string map into typed SessionOptions.
// Empty string = inherit (nil). Unknown keys are an error — keeps the API
// strict when option sets evolve.
func parseOptions(in map[string]string) (store.SessionOptions, error) {
	var o store.SessionOptions
	for k, raw := range in {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		switch k {
		case "termType":
			o.TermType = &v
		case "keepaliveInterval":
			d, err := time.ParseDuration(v)
			if err != nil || d < 0 {
				return o, fmt.Errorf("app: keepaliveInterval %q: want a duration like 30s", v)
			}
			o.KeepaliveInterval = &d
		case "reconnect":
			if v != "off" && v != "prompt" && v != "auto" {
				return o, fmt.Errorf("app: reconnect %q: want off|prompt|auto", v)
			}
			o.Reconnect = &v
		case "theme":
			o.ThemeRef = &v
		case "scrollbackLines":
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				return o, fmt.Errorf("app: scrollbackLines %q: want a positive integer", v)
			}
			o.ScrollbackLines = &n
		case "auditScope":
			if v != "off" && v != "events" && v != "events+input" && v != "full-io" {
				return o, fmt.Errorf("app: auditScope %q: want off|events|events+input|full-io", v)
			}
			o.AuditScope = &v
		case "keyFile":
			o.KeyFile = &v
		case "useAgent":
			if v != "true" && v != "false" {
				return o, fmt.Errorf("app: useAgent %q: want true|false", v)
			}
			b := v == "true"
			o.UseAgent = &b
		case "socksPort":
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 65535 {
				return o, fmt.Errorf("app: socksPort %q: want a port 1-65535", v)
			}
			o.SocksPort = &n
		case "socksOnly":
			if v != "true" && v != "false" {
				return o, fmt.Errorf("app: socksOnly %q: want true|false", v)
			}
			b := v == "true"
			o.SocksOnly = &b
		default:
			return o, fmt.Errorf("app: unknown option %q", k)
		}
	}
	return o, nil
}

func (a *App) sessionIDByName(folderID, name string) (string, error) {
	for _, s := range a.st.Sessions() {
		if s.FolderID == folderID && strings.EqualFold(s.Name, name) {
			return s.ID, nil
		}
	}
	return "", fmt.Errorf("app: session %q not found after save", name)
}

func strPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func durPtr(p *time.Duration) string {
	if p == nil {
		return ""
	}
	return p.String()
}

func intPtr(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}

func boolPtr(p *bool) string {
	if p == nil {
		return ""
	}
	if *p {
		return "true"
	}
	return "false"
}

// storeRoot mirrors the CLI's resolution (F9_STORE override, XDG default).
func storeRoot() (string, error) {
	if p := os.Getenv("F9_STORE"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("app: resolve home: %w", err)
	}
	return filepath.Join(home, ".config", "f9", "sessions"), nil
}
