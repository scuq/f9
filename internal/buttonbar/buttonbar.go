// Package buttonbar is the per-folder button-bar store. A bar is rows of user
// buttons {icon, label, color, action}. Actions: send (pongo2 template),
// snippet (by id), launch (exec argv, no shell), url, internal (UI command).
// Resolution is an inheritable override: the nearest folder up the chain that
// defines a bar wins wholesale, else the global bar, else empty — so bars
// switch with the session folder.
//
// Persistence: global.yaml and folder/<folderID>.yaml under <dir>, atomic writes.
package buttonbar

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type Action struct {
	Kind      string   `yaml:"kind" json:"kind"` // send|snippet|launch|url|internal
	Text      string   `yaml:"text,omitempty" json:"text"`
	SnippetID string   `yaml:"snippet,omitempty" json:"snippetId"`
	Args      []string `yaml:"args,omitempty" json:"args"`
	DelayMs   int      `yaml:"delayMs,omitempty" json:"delayMs"`
	Bracketed bool     `yaml:"bracketed,omitempty" json:"bracketed"`
}

type Button struct {
	Icon   string `yaml:"icon,omitempty" json:"icon"`
	Label  string `yaml:"label" json:"label"`
	Color  string `yaml:"color,omitempty" json:"color"`
	Action Action `yaml:"action" json:"action"`
}

type Row struct {
	Buttons []Button `yaml:"buttons" json:"buttons"`
}

type Bar struct {
	Rows []Row `yaml:"rows" json:"rows"`
}

// ChainFunc returns folder IDs root -> leaf (inclusive) for a folder.
type ChainFunc func(folderID string) []string

var actionKinds = map[string]bool{"send": true, "snippet": true, "launch": true, "url": true, "internal": true}

const (
	globalFile = "global.yaml"
	folderDir  = "folder"
)

func validateBar(b Bar) error {
	for ri, r := range b.Rows {
		for bi, btn := range r.Buttons {
			if strings.TrimSpace(btn.Label) == "" {
				return fmt.Errorf("buttonbar: row %d button %d: empty label", ri, bi)
			}
			if !actionKinds[btn.Action.Kind] {
				return fmt.Errorf("buttonbar: row %d button %d: invalid action kind %q", ri, bi, btn.Action.Kind)
			}
		}
	}
	return nil
}

// YAMLStore is the file-backed button-bar store.
type YAMLStore struct {
	dir   string
	chain ChainFunc

	mu     sync.RWMutex
	global *Bar
	folder map[string]*Bar
}

// Open loads (creating if necessary) a button-bar store rooted at dir.
func Open(dir string, chain ChainFunc) (*YAMLStore, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("buttonbar: resolve dir: %w", err)
	}
	for _, d := range []string{abs, filepath.Join(abs, folderDir)} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, fmt.Errorf("buttonbar: create dir: %w", err)
		}
	}
	s := &YAMLStore{dir: abs, chain: chain, folder: map[string]*Bar{}}
	if err := s.reload(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *YAMLStore) reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.global = nil
	s.folder = map[string]*Bar{}

	if b, ok, err := readBar(filepath.Join(s.dir, globalFile)); err != nil {
		return err
	} else if ok {
		s.global = &b
	}
	entries, err := os.ReadDir(filepath.Join(s.dir, folderDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("buttonbar: read folder dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".yaml")
		if b, ok, err := readBar(filepath.Join(s.dir, folderDir, e.Name())); err != nil {
			return err
		} else if ok {
			bar := b
			s.folder[id] = &bar
		}
	}
	return nil
}

func (s *YAMLStore) chainFor(folderID string) []string {
	if folderID == "" {
		return nil
	}
	if s.chain != nil {
		return s.chain(folderID)
	}
	return []string{folderID}
}

// Resolve returns the effective bar for a folder: nearest defined bar up the
// chain (leaf -> root), else global, else empty.
func (s *YAMLStore) Resolve(folderID string) Bar {
	s.mu.RLock()
	defer s.mu.RUnlock()
	chain := s.chainFor(folderID)
	for i := len(chain) - 1; i >= 0; i-- {
		if b, ok := s.folder[chain[i]]; ok {
			return *b
		}
	}
	if s.global != nil {
		return *s.global
	}
	return Bar{}
}

// Get returns a scope's own bar (folderID "" = global) and whether it is defined.
func (s *YAMLStore) Get(folderID string) (Bar, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if folderID == "" {
		if s.global != nil {
			return *s.global, true
		}
		return Bar{}, false
	}
	if b, ok := s.folder[folderID]; ok {
		return *b, true
	}
	return Bar{}, false
}

// Save writes a scope's own bar (folderID "" = global).
func (s *YAMLStore) Save(folderID string, bar Bar) error {
	if err := validateBar(bar); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.pathFor(folderID)
	if err := writeBar(path, bar); err != nil {
		return err
	}
	b := bar
	if folderID == "" {
		s.global = &b
	} else {
		s.folder[folderID] = &b
	}
	return nil
}

// Delete removes a scope's own bar (reverting it to the inherited/global bar).
func (s *YAMLStore) Delete(folderID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.pathFor(folderID)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("buttonbar: remove %s: %w", path, err)
	}
	if folderID == "" {
		s.global = nil
	} else {
		delete(s.folder, folderID)
	}
	return nil
}

// Export returns the YAML of a scope's own bar.
func (s *YAMLStore) Export(folderID string) (string, error) {
	b, ok := s.Get(folderID)
	if !ok {
		return "", fmt.Errorf("buttonbar: no bar defined for this scope")
	}
	out, err := yaml.Marshal(b)
	if err != nil {
		return "", fmt.Errorf("buttonbar: marshal: %w", err)
	}
	return string(out), nil
}

// Import parses YAML and saves it as a scope's own bar.
func (s *YAMLStore) Import(folderID, yamlText string) error {
	var b Bar
	if err := yaml.Unmarshal([]byte(yamlText), &b); err != nil {
		return fmt.Errorf("buttonbar: parse import: %w", err)
	}
	return s.Save(folderID, b)
}

func (s *YAMLStore) pathFor(folderID string) string {
	if folderID == "" {
		return filepath.Join(s.dir, globalFile)
	}
	return filepath.Join(s.dir, folderDir, folderID+".yaml")
}

func readBar(path string) (Bar, bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Bar{}, false, nil
		}
		return Bar{}, false, fmt.Errorf("buttonbar: read %s: %w", path, err)
	}
	var bar Bar
	if err := yaml.Unmarshal(b, &bar); err != nil {
		return Bar{}, false, fmt.Errorf("buttonbar: parse %s: %w", path, err)
	}
	return bar, true, nil
}

func writeBar(path string, bar Bar) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".f9bar-*")
	if err != nil {
		return fmt.Errorf("buttonbar: temp: %w", err)
	}
	tmpName := tmp.Name()
	enc := yaml.NewEncoder(tmp)
	enc.SetIndent(2)
	if err := enc.Encode(bar); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("buttonbar: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("buttonbar: flush: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("buttonbar: close: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("buttonbar: rename: %w", err)
	}
	return nil
}
