// Package vars is the scoped variable store (global -> folder -> session, same
// resolution semantics as session options; see internal/store resolve.go).
// Secrets are rejected by key-naming policy (ADR-0005). Consumers: snippet
// templating (pongo2), button-bar send-strings, and Lua read access (phase 07).
//
// OS tagging: a value is either a scalar (applies to all OS families) or an OS
// map keyed by "all", "unknown", or an osdetect family (linux, ios, nxos, ...).
// Per key, resolution selects: for a detected family F, entry[F] else
// entry["all"]; for an undetected session, entry["unknown"] else entry["all"];
// otherwise the key is absent for that session. Scope overlay (global -> folder
// chain -> session) then runs on the selected values.
//
// Persistence: one YAML file per scope under <dir>:
//
//	global.yaml, folder/<folderID>.yaml, session/<sessionID>.yaml
//
// A scalar key emits as a bare scalar; an OS-mapped key emits as a mapping.
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

// Store is the scoped, OS-aware variable store. family is the session's
// detected OS ("" = undetected); os is the selector to write ("all" default).
type Store interface {
	Get(s Scope, key, family string) (string, bool)
	List(s Scope, family string) map[string]string
	Put(s Scope, key, value, os string) error
	Delete(s Scope, key, os string) error
}

// ChainFunc returns folder IDs root -> leaf (inclusive) for a folder; nil means
// no ancestor inheritance.
type ChainFunc func(folderID string) []string

var keyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var secretMarkers = []string{"password", "passwd", "secret", "token", "apikey", "api_key", "privatekey", "private_key"}

// allowedOS is the set of valid OS selectors for Put/Delete.
var allowedOS = map[string]bool{
	"all": true, "unknown": true,
	"linux": true, "openbsd": true, "ios": true, "nxos": true,
	"panos": true, "junos": true, "windows": true,
}

const (
	globalFile = "global.yaml"
	folderDir  = "folder"
	sessionDir = "session"
	selAll     = "all"
	selUnknown = "unknown"
)

// osValue is one variable's value: a selector ("all"/"unknown"/<family>) -> value
// map. A bare scalar in YAML is stored under "all".
type osValue struct {
	m map[string]string
}

func (v osValue) MarshalYAML() (interface{}, error) {
	if len(v.m) == 1 {
		if s, ok := v.m[selAll]; ok {
			return s, nil // emit a bare scalar
		}
	}
	return v.m, nil
}

func (v *osValue) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		v.m = map[string]string{selAll: node.Value}
		return nil
	case yaml.MappingNode:
		mm := map[string]string{}
		if err := node.Decode(&mm); err != nil {
			return err
		}
		v.m = mm
		return nil
	default:
		return fmt.Errorf("vars: unexpected yaml node kind %d", node.Kind)
	}
}

// selectOS resolves one osValue for a session's family ("" = undetected).
func selectOS(v osValue, family string) (string, bool) {
	if family != "" {
		if val, ok := v.m[family]; ok {
			return val, true
		}
	} else if val, ok := v.m[selUnknown]; ok {
		return val, true
	}
	if val, ok := v.m[selAll]; ok {
		return val, true
	}
	return "", false
}

// YAMLStore is the file-backed vars store.
type YAMLStore struct {
	dir   string
	chain ChainFunc

	mu      sync.RWMutex
	global  map[string]osValue
	folder  map[string]map[string]osValue
	session map[string]map[string]osValue
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
	s.global = map[string]osValue{}
	s.folder = map[string]map[string]osValue{}
	s.session = map[string]map[string]osValue{}

	m, err := readScope(filepath.Join(s.dir, globalFile))
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

func (s *YAMLStore) loadScopeDir(sub string, into map[string]map[string]osValue) error {
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
		m, err := readScope(filepath.Join(dir, e.Name()))
		if err != nil {
			return err
		}
		if m != nil {
			into[id] = m
		}
	}
	return nil
}

func (s *YAMLStore) List(sc Scope, family string) map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string]string{}
	apply := func(m map[string]osValue) {
		for k, v := range m {
			if val, ok := selectOS(v, family); ok {
				out[k] = val
			}
		}
	}
	apply(s.global)
	for _, fid := range s.folderChainFor(sc.FolderID) {
		apply(s.folder[fid])
	}
	if sc.SessionID != "" {
		apply(s.session[sc.SessionID])
	}
	return out
}

func (s *YAMLStore) folderChainFor(folderID string) []string {
	if folderID == "" {
		return nil
	}
	if s.chain != nil {
		return s.chain(folderID)
	}
	return []string{folderID}
}

func (s *YAMLStore) Get(sc Scope, key, family string) (string, bool) {
	v, ok := s.List(sc, family)[key]
	return v, ok
}

func (s *YAMLStore) Put(sc Scope, key, value, osSel string) error {
	if !keyPattern.MatchString(key) {
		return fmt.Errorf("vars: invalid key %q (want ^[A-Za-z_][A-Za-z0-9_]*$)", key)
	}
	if IsSecretKey(key) {
		return fmt.Errorf("vars: key %q looks like a secret; keep secrets in the SSH agent or an interactive prompt (ADR-0005)", key)
	}
	if osSel == "" {
		osSel = selAll
	}
	if !allowedOS[osSel] {
		return fmt.Errorf("vars: invalid OS selector %q (want all|unknown|<family>)", osSel)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m, path := s.scopeMapLocked(sc, true)
	v := m[key]
	if v.m == nil {
		v.m = map[string]string{}
	}
	v.m[osSel] = value
	m[key] = v
	return writeScope(path, m)
}

// Delete removes an OS selector from a key (osSel != ""), or the entire key
// (osSel == ""). Empty keys and empty scope files are removed.
func (s *YAMLStore) Delete(sc Scope, key, osSel string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, path := s.scopeMapLocked(sc, false)
	if m == nil {
		return nil
	}
	v, ok := m[key]
	if !ok {
		return nil
	}
	if osSel == "" {
		delete(m, key)
	} else {
		delete(v.m, osSel)
		if len(v.m) == 0 {
			delete(m, key)
		} else {
			m[key] = v
		}
	}
	if len(m) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("vars: remove %s: %w", path, err)
		}
		return nil
	}
	return writeScope(path, m)
}

func (s *YAMLStore) scopeMapLocked(sc Scope, create bool) (map[string]osValue, string) {
	switch {
	case sc.SessionID != "":
		m := s.session[sc.SessionID]
		if m == nil && create {
			m = map[string]osValue{}
			s.session[sc.SessionID] = m
		}
		return m, filepath.Join(s.dir, sessionDir, sc.SessionID+".yaml")
	case sc.FolderID != "":
		m := s.folder[sc.FolderID]
		if m == nil && create {
			m = map[string]osValue{}
			s.folder[sc.FolderID] = m
		}
		return m, filepath.Join(s.dir, folderDir, sc.FolderID+".yaml")
	default:
		if s.global == nil && create {
			s.global = map[string]osValue{}
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

func readScope(path string) (map[string]osValue, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("vars: read %s: %w", path, err)
	}
	m := map[string]osValue{}
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("vars: parse %s: %w", path, err)
	}
	return m, nil
}

// writeScope writes m atomically (temp file + rename).
func writeScope(path string, m map[string]osValue) error {
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
