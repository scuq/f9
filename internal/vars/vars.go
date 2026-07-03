// Package vars is the scoped variable store (global -> folder -> session, same
// resolution semantics as session options; see internal/store resolve.go).
// Secrets are rejected by key-naming policy (ADR-0005); use the SSH agent or an
// interactive prompt instead. Consumers: snippet templating (pongo2),
// button-bar send-strings, and Lua read access (phase 07).
//
// Persistence: one YAML file per scope under <dir>:
//
//	global.yaml            map[key]value at global scope
//	folder/<folderID>.yaml map[key]value at a folder scope
//	session/<sessionID>.yaml map[key]value at a session scope
//
// yaml.v3 sorts map keys on encode, so files stay git-friendly.
package vars

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Scope selects a level in the global -> folder -> session hierarchy.
type Scope struct {
	FolderID  string // "" = global
	SessionID string // "" = folder/global level
}

// Store is the scoped variable store.
type Store interface {
	Get(s Scope, key string) (string, bool)
	// List returns the fully resolved view: global overlaid by the folder chain
	// (root -> leaf) overlaid by the session.
	List(s Scope) map[string]string
	Put(s Scope, key, value string) error
	Delete(s Scope, key string) error
}

// ChainFunc returns the folder IDs from the root folder down to folderID
// (inclusive, root first) — how the vars store learns the folder hierarchy for
// resolution. The session store provides it; nil means "no ancestor
// inheritance" (only the exact folder scope is consulted).
type ChainFunc func(folderID string) []string

// keyPattern accepts jinja/pongo2-style identifiers so vars map 1:1 to
// template variables.
var keyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// secretMarkers: a key whose lowercased name contains any of these is rejected.
var secretMarkers = []string{"password", "passwd", "secret", "token", "apikey", "api_key", "privatekey", "private_key"}

const (
	globalFile = "global.yaml"
	folderDir  = "folder"
	sessionDir = "session"
)

// YAMLStore is the file-backed vars store.
type YAMLStore struct {
	dir   string
	chain ChainFunc

	mu      sync.RWMutex
	global  map[string]string
	folder  map[string]map[string]string
	session map[string]map[string]string
}

var _ Store = (*YAMLStore)(nil)

// Open loads (creating if necessary) a vars store rooted at dir.
func Open(dir string, chain ChainFunc) (*YAMLStore, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("vars: resolve dir: %w", err)
	}
	for _, d := range []string{abs, filepath.Join(abs, folderDir), filepath.Join(abs, sessionDir)} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, fmt.Errorf("vars: create dir: %w", err)
		}
	}
	s := &YAMLStore{dir: abs, chain: chain}
	if err := s.reload(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *YAMLStore) reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.global = map[string]string{}
	s.folder = map[string]map[string]string{}
	s.session = map[string]map[string]string{}

	m, err := readMap(filepath.Join(s.dir, globalFile))
	if err != nil {
		return err
	}
	if m != nil {
		s.global = m
	}
	if err := s.loadScopeDir(folderDir, s.folder); err != nil {
		return err
	}
	return s.loadScopeDir(sessionDir, s.session)
}

func (s *YAMLStore) loadScopeDir(sub string, into map[string]map[string]string) error {
	dir := filepath.Join(s.dir, sub)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("vars: read %s: %w", sub, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".yaml")
		m, err := readMap(filepath.Join(dir, e.Name()))
		if err != nil {
			return err
		}
		if m != nil {
			into[id] = m
		}
	}
	return nil
}

func (s *YAMLStore) List(sc Scope) map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string]string{}
	for k, v := range s.global {
		out[k] = v
	}
	for _, fid := range s.folderChainFor(sc.FolderID) {
		for k, v := range s.folder[fid] {
			out[k] = v
		}
	}
	if sc.SessionID != "" {
		for k, v := range s.session[sc.SessionID] {
			out[k] = v
		}
	}
	return out
}

// folderChainFor returns root->leaf folder IDs for folderID via the chain func,
// or just [folderID] when no chain is configured.
func (s *YAMLStore) folderChainFor(folderID string) []string {
	if folderID == "" {
		return nil
	}
	if s.chain != nil {
		return s.chain(folderID)
	}
	return []string{folderID}
}

func (s *YAMLStore) Get(sc Scope, key string) (string, bool) {
	v, ok := s.List(sc)[key]
	return v, ok
}

func (s *YAMLStore) Put(sc Scope, key, value string) error {
	if !keyPattern.MatchString(key) {
		return fmt.Errorf("vars: invalid key %q (want ^[A-Za-z_][A-Za-z0-9_]*$)", key)
	}
	if IsSecretKey(key) {
		return fmt.Errorf("vars: key %q looks like a secret; keep secrets in the SSH agent or an interactive prompt (ADR-0005)", key)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m, path := s.scopeMapLocked(sc, true)
	m[key] = value
	return writeMap(path, m)
}

func (s *YAMLStore) Delete(sc Scope, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, path := s.scopeMapLocked(sc, false)
	if m == nil {
		return nil
	}
	if _, ok := m[key]; !ok {
		return nil
	}
	delete(m, key)
	if len(m) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("vars: remove %s: %w", path, err)
		}
		return nil
	}
	return writeMap(path, m)
}

// scopeMapLocked returns the writable map and file path for the most specific
// non-empty scope level (session > folder > global). With create, a nil map is
// initialized and registered. Callers must hold s.mu.
func (s *YAMLStore) scopeMapLocked(sc Scope, create bool) (map[string]string, string) {
	switch {
	case sc.SessionID != "":
		m := s.session[sc.SessionID]
		if m == nil && create {
			m = map[string]string{}
			s.session[sc.SessionID] = m
		}
		return m, filepath.Join(s.dir, sessionDir, sc.SessionID+".yaml")
	case sc.FolderID != "":
		m := s.folder[sc.FolderID]
		if m == nil && create {
			m = map[string]string{}
			s.folder[sc.FolderID] = m
		}
		return m, filepath.Join(s.dir, folderDir, sc.FolderID+".yaml")
	default:
		if s.global == nil && create {
			s.global = map[string]string{}
		}
		return s.global, filepath.Join(s.dir, globalFile)
	}
}

// IsSecretKey reports whether a key name is rejected by the secret policy.
func IsSecretKey(key string) bool {
	lk := strings.ToLower(key)
	for _, m := range secretMarkers {
		if strings.Contains(lk, m) {
			return true
		}
	}
	return false
}

func readMap(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("vars: read %s: %w", path, err)
	}
	m := map[string]string{}
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("vars: parse %s: %w", path, err)
	}
	return m, nil
}

// writeMap writes m atomically (temp file + rename), mirroring internal/store.
func writeMap(path string, m map[string]string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".f9vars-*")
	if err != nil {
		return fmt.Errorf("vars: temp: %w", err)
	}
	tmpName := tmp.Name()
	enc := yaml.NewEncoder(tmp)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("vars: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("vars: flush: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("vars: close: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("vars: rename: %w", err)
	}
	return nil
}
