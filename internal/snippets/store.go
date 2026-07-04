package snippets

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Folder is a node in the standalone snippet library tree (independent of the
// session tree).
type Folder struct {
	ID       string `yaml:"id" json:"id"`
	ParentID string `yaml:"parent,omitempty" json:"parentId"`
	Name     string `yaml:"name" json:"name"`
}

// Store is the file-backed snippet library: folders.yaml plus one item file per
// snippet under items/.
type Store struct {
	dir string

	mu       sync.RWMutex
	folders  map[string]Folder
	snippets map[string]Snippet
}

const (
	foldersFile = "folders.yaml"
	itemsDir    = "items"
)

func newID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Open loads (creating if necessary) a snippet store rooted at dir.
func Open(dir string) (*Store, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("snippets: dir: %w", err)
	}
	for _, d := range []string{abs, filepath.Join(abs, itemsDir)} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, fmt.Errorf("snippets: mkdir: %w", err)
		}
	}
	s := &Store{dir: abs, folders: map[string]Folder{}, snippets: map[string]Snippet{}}
	if err := s.reload(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.folders = map[string]Folder{}
	s.snippets = map[string]Snippet{}

	if b, err := os.ReadFile(filepath.Join(s.dir, foldersFile)); err == nil {
		var fl []Folder
		if err := yaml.Unmarshal(b, &fl); err != nil {
			return fmt.Errorf("snippets: parse folders: %w", err)
		}
		for _, f := range fl {
			if f.ID != "" {
				s.folders[f.ID] = f
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("snippets: read folders: %w", err)
	}

	entries, err := os.ReadDir(filepath.Join(s.dir, itemsDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("snippets: read items: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.dir, itemsDir, e.Name()))
		if err != nil {
			return fmt.Errorf("snippets: read item: %w", err)
		}
		var sn Snippet
		if err := yaml.Unmarshal(b, &sn); err != nil {
			return fmt.Errorf("snippets: parse item %s: %w", e.Name(), err)
		}
		if sn.ID != "" {
			s.snippets[sn.ID] = sn
		}
	}
	return nil
}

// Folders returns all folders, sorted by name.
func (s *Store) Folders() []Folder {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Folder, 0, len(s.folders))
	for _, f := range s.folders {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// List returns all snippets, sorted by name.
func (s *Store) List() []Snippet {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Snippet, 0, len(s.snippets))
	for _, sn := range s.snippets {
		out = append(out, sn)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (s *Store) Get(id string) (Snippet, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sn, ok := s.snippets[id]
	return sn, ok
}

// SaveFolder creates or updates a folder (assigns an ID if empty).
func (s *Store) SaveFolder(f Folder) (Folder, error) {
	if strings.TrimSpace(f.Name) == "" {
		return Folder{}, fmt.Errorf("snippets: folder name required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if f.ID == "" {
		f.ID = newID()
	}
	if f.ParentID != "" && f.ParentID != f.ID {
		if _, ok := s.folders[f.ParentID]; !ok {
			return Folder{}, fmt.Errorf("snippets: parent folder not found")
		}
	}
	s.folders[f.ID] = f
	if err := s.writeFoldersLocked(); err != nil {
		return Folder{}, err
	}
	return f, nil
}

// DeleteFolder removes an empty folder (rejects if it has subfolders or snippets).
func (s *Store) DeleteFolder(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.folders[id]; !ok {
		return nil
	}
	for _, f := range s.folders {
		if f.ParentID == id {
			return fmt.Errorf("snippets: folder not empty (has subfolders)")
		}
	}
	for _, sn := range s.snippets {
		if sn.FolderID == id {
			return fmt.Errorf("snippets: folder not empty (has snippets)")
		}
	}
	delete(s.folders, id)
	return s.writeFoldersLocked()
}

// SaveSnippet creates or updates a snippet (assigns an ID if empty).
func (s *Store) SaveSnippet(sn Snippet) (Snippet, error) {
	if strings.TrimSpace(sn.Name) == "" {
		return Snippet{}, fmt.Errorf("snippets: name required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if sn.FolderID != "" {
		if _, ok := s.folders[sn.FolderID]; !ok {
			return Snippet{}, fmt.Errorf("snippets: folder not found")
		}
	}
	if sn.ID == "" {
		sn.ID = newID()
	}
	s.snippets[sn.ID] = sn
	if err := writeYAMLFile(filepath.Join(s.dir, itemsDir, sn.ID+".yaml"), sn); err != nil {
		return Snippet{}, err
	}
	return sn, nil
}

func (s *Store) DeleteSnippet(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.snippets[id]; !ok {
		return nil
	}
	delete(s.snippets, id)
	path := filepath.Join(s.dir, itemsDir, id+".yaml")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("snippets: remove: %w", err)
	}
	return nil
}

func (s *Store) writeFoldersLocked() error {
	fl := make([]Folder, 0, len(s.folders))
	for _, f := range s.folders {
		fl = append(fl, f)
	}
	sort.Slice(fl, func(i, j int) bool {
		if fl[i].Name != fl[j].Name {
			return fl[i].Name < fl[j].Name
		}
		return fl[i].ID < fl[j].ID
	})
	return writeYAMLFile(filepath.Join(s.dir, foldersFile), fl)
}

func writeYAMLFile(path string, v interface{}) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".f9snip-*")
	if err != nil {
		return fmt.Errorf("snippets: temp: %w", err)
	}
	tmpName := tmp.Name()
	enc := yaml.NewEncoder(tmp)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("snippets: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("snippets: flush: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("snippets: close: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("snippets: rename: %w", err)
	}
	return nil
}
