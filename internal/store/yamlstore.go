package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	folderFile = "folder.yaml"
	metaDir    = ".meta"
)

// ErrNotFound is returned when an ID does not resolve to a stored object.
var ErrNotFound = errors.New("store: not found")

// YAMLStore implements Store over a directory tree of YAML files: every
// directory is a Folder (described by its folder.yaml), every other *.yaml
// file in it is a Session. Directory structure is authoritative for
// parent/child relations. Machine-written metadata lives in <root>/.meta/.
// The root folder's options act as the "Default Session" options.
type YAMLStore struct {
	root string

	mu          sync.RWMutex
	rootID      string
	folders     map[string]*Folder
	sessions    map[string]*Session
	folderDir   map[string]string
	sessionFile map[string]string
}

var _ Store = (*YAMLStore)(nil)

// Open loads (creating if necessary) a store rooted at dir.
func Open(dir string) (*YAMLStore, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("store: resolve root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("store: create root: %w", err)
	}
	s := &YAMLStore{root: abs}
	if err := s.LoadAll(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *YAMLStore) LoadAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reload()
}

// reload rebuilds all in-memory state from disk. Callers must hold s.mu.
func (s *YAMLStore) reload() error {
	s.folders = map[string]*Folder{}
	s.sessions = map[string]*Session{}
	s.folderDir = map[string]string{}
	s.sessionFile = map[string]string{}

	root, err := s.loadOrInitFolder(s.root, "", "Sessions")
	if err != nil {
		return err
	}
	s.rootID = root.ID
	return s.loadDir(s.root, root.ID)
}

func (s *YAMLStore) loadOrInitFolder(dir, parentID, fallbackName string) (*Folder, error) {
	path := filepath.Join(dir, folderFile)
	var f Folder
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := yaml.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("store: parse %s: %w", path, err)
		}
	case os.IsNotExist(err):
		f = Folder{ID: NewULID(), Name: fallbackName, Revision: 1, UpdatedAt: time.Now().UTC()}
		if err := writeYAML(path, &f); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("store: read %s: %w", path, err)
	}
	if f.ID == "" {
		f.ID = NewULID()
		if err := writeYAML(path, &f); err != nil {
			return nil, err
		}
	}
	f.ParentID = parentID // directory structure is authoritative
	if f.Name == "" {
		f.Name = fallbackName
	}
	if _, dup := s.folders[f.ID]; dup {
		return nil, fmt.Errorf("store: duplicate folder id %s at %s", f.ID, dir)
	}
	cp := f
	s.folders[f.ID] = &cp
	s.folderDir[f.ID] = dir
	return &cp, nil
}

func (s *YAMLStore) loadDir(dir, folderID string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("store: read dir %s: %w", dir, err)
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		sub := filepath.Join(dir, name)
		if e.IsDir() {
			child, err := s.loadOrInitFolder(sub, folderID, name)
			if err != nil {
				return err
			}
			if err := s.loadDir(sub, child.ID); err != nil {
				return err
			}
			continue
		}
		if name == folderFile || !strings.HasSuffix(name, ".yaml") {
			continue
		}
		if err := s.loadSessionFile(sub, folderID); err != nil {
			return err
		}
	}
	return nil
}

func (s *YAMLStore) loadSessionFile(path, folderID string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("store: read %s: %w", path, err)
	}
	var sess Session
	if err := yaml.Unmarshal(data, &sess); err != nil {
		return fmt.Errorf("store: parse %s: %w", path, err)
	}
	if sess.ID == "" { // auto-heal hand-created files
		sess.ID = NewULID()
		if err := writeYAML(path, &sess); err != nil {
			return err
		}
	}
	sess.FolderID = folderID
	if sess.Name == "" {
		sess.Name = strings.TrimSuffix(filepath.Base(path), ".yaml")
	}
	if _, dup := s.sessions[sess.ID]; dup {
		return fmt.Errorf("store: duplicate session id %s at %s", sess.ID, path)
	}
	cp := sess
	s.sessions[sess.ID] = &cp
	s.sessionFile[sess.ID] = path
	return nil
}

// RootID returns the ID of the implicit root folder ("Default Session" scope).
func (s *YAMLStore) RootID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rootID
}

func (s *YAMLStore) Folders() []Folder {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Folder, 0, len(s.folders))
	for _, f := range s.folders {
		out = append(out, *f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ParentID != out[j].ParentID {
			return out[i].ParentID < out[j].ParentID
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (s *YAMLStore) Sessions() []Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, *sess)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FolderID != out[j].FolderID {
			return out[i].FolderID < out[j].FolderID
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Resolve returns the session plus its effective options: root folder options
// (defaults), overlaid by each folder down the chain, overlaid by the session.
func (s *YAMLStore) Resolve(sessionID string) (Session, SessionOptions, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		return Session{}, SessionOptions{}, fmt.Errorf("store: session %s: %w", sessionID, ErrNotFound)
	}
	chain, err := s.folderChain(sess.FolderID)
	if err != nil {
		return Session{}, SessionOptions{}, err
	}
	var eff SessionOptions
	for _, f := range chain {
		eff = overlay(eff, f.Options)
	}
	eff = overlay(eff, sess.Options)
	return *sess, eff, nil
}

// folderChain returns root-first chain. Callers must hold s.mu (read or write).
func (s *YAMLStore) folderChain(folderID string) ([]*Folder, error) {
	var rev []*Folder
	id := folderID
	for id != "" {
		f, ok := s.folders[id]
		if !ok {
			return nil, fmt.Errorf("store: folder %s: %w", id, ErrNotFound)
		}
		rev = append(rev, f)
		id = f.ParentID
		if len(rev) > 1000 {
			return nil, errors.New("store: folder chain too deep or cyclic")
		}
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev, nil
}

// Put creates (empty ID) or updates a session, bumping Revision and UpdatedAt.
// Session names are unique per folder, case-insensitively (filesystems!).
func (s *YAMLStore) Put(in Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(in.Name) == "" {
		return errors.New("store: session name required")
	}
	if in.FolderID == "" {
		in.FolderID = s.rootID
	}
	dir, ok := s.folderDir[in.FolderID]
	if !ok {
		return fmt.Errorf("store: folder %s: %w", in.FolderID, ErrNotFound)
	}
	fn, err := sessionFileName(in.Name)
	if err != nil {
		return err
	}
	target := filepath.Join(dir, fn)

	for id, other := range s.sessions {
		if id == in.ID || other.FolderID != in.FolderID {
			continue
		}
		if strings.EqualFold(other.Name, in.Name) || s.sessionFile[id] == target {
			return fmt.Errorf("store: session name %q already exists in folder", in.Name)
		}
	}

	var oldPath string
	if in.ID == "" {
		in.ID = NewULID()
		in.Revision = 1
	} else if old, ok := s.sessions[in.ID]; ok {
		in.Revision = old.Revision + 1
		oldPath = s.sessionFile[in.ID]
	} else if in.Revision == 0 {
		in.Revision = 1
	}
	in.UpdatedAt = time.Now().UTC()
	if in.Proto == "" {
		in.Proto = "ssh"
	}

	if err := writeYAML(target, &in); err != nil {
		return err
	}
	if oldPath != "" && oldPath != target {
		if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("store: remove renamed session file: %w", err)
		}
	}
	cp := in
	s.sessions[in.ID] = &cp
	s.sessionFile[in.ID] = target
	return nil
}

// PutFolder creates (empty ID) or updates a folder. Updating the root folder
// updates the store-wide default options. Rename/move re-locates the directory.
func (s *YAMLStore) PutFolder(in Folder) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if in.ID != "" && in.ID == s.rootID {
		old := s.folders[s.rootID]
		in.ParentID = ""
		in.Name = old.Name
		in.Revision = old.Revision + 1
		in.UpdatedAt = time.Now().UTC()
		if err := writeYAML(filepath.Join(s.root, folderFile), &in); err != nil {
			return err
		}
		return s.reload()
	}

	if strings.TrimSpace(in.Name) == "" {
		return errors.New("store: folder name required")
	}
	if in.ParentID == "" {
		in.ParentID = s.rootID
	}
	parentDir, ok := s.folderDir[in.ParentID]
	if !ok {
		return fmt.Errorf("store: parent folder %s: %w", in.ParentID, ErrNotFound)
	}
	dirName := sanitizeName(in.Name)
	if dirName == "" {
		return fmt.Errorf("store: name %q sanitizes to empty", in.Name)
	}
	target := filepath.Join(parentDir, dirName)

	for id, other := range s.folders {
		if id == in.ID || other.ParentID != in.ParentID {
			continue
		}
		if strings.EqualFold(other.Name, in.Name) || s.folderDir[id] == target {
			return fmt.Errorf("store: folder name %q already exists under parent", in.Name)
		}
	}

	if in.ID == "" {
		in.ID = NewULID()
		in.Revision = 1
		in.UpdatedAt = time.Now().UTC()
		if err := os.MkdirAll(target, 0o700); err != nil {
			return fmt.Errorf("store: create folder dir: %w", err)
		}
		if err := writeYAML(filepath.Join(target, folderFile), &in); err != nil {
			return err
		}
		return s.reload()
	}

	old, ok := s.folders[in.ID]
	if !ok {
		return fmt.Errorf("store: folder %s: %w", in.ID, ErrNotFound)
	}
	oldDir := s.folderDir[in.ID]
	in.Revision = old.Revision + 1
	in.UpdatedAt = time.Now().UTC()
	if oldDir != target {
		sep := string(filepath.Separator)
		if strings.HasPrefix(target+sep, oldDir+sep) {
			return errors.New("store: cannot move folder into its own subtree")
		}
		if err := os.Rename(oldDir, target); err != nil {
			return fmt.Errorf("store: move folder: %w", err)
		}
	}
	if err := writeYAML(filepath.Join(target, folderFile), &in); err != nil {
		return err
	}
	return s.reload()
}

// Delete removes a session (plus its meta sidecar) or an empty folder.
func (s *YAMLStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[id]; ok {
		if err := os.Remove(s.sessionFile[id]); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("store: delete session file: %w", err)
		}
		_ = os.Remove(filepath.Join(s.root, metaDir, id+".yaml"))
		delete(s.sessions, id)
		delete(s.sessionFile, id)
		return nil
	}
	if _, ok := s.folders[id]; ok {
		if id == s.rootID {
			return errors.New("store: cannot delete root folder")
		}
		for _, f := range s.folders {
			if f.ParentID == id {
				return errors.New("store: folder not empty (contains folders)")
			}
		}
		for _, sess := range s.sessions {
			if sess.FolderID == id {
				return errors.New("store: folder not empty (contains sessions)")
			}
		}
		dir := s.folderDir[id]
		if err := os.Remove(filepath.Join(dir, folderFile)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("store: delete folder.yaml: %w", err)
		}
		if err := os.Remove(dir); err != nil {
			return fmt.Errorf("store: delete folder dir: %w", err)
		}
		delete(s.folders, id)
		delete(s.folderDir, id)
		return nil
	}
	return fmt.Errorf("store: id %s: %w", id, ErrNotFound)
}

// Meta returns the machine-written sidecar for a session; zero-value (with
// SessionID set) if none exists yet.
func (s *YAMLStore) Meta(sessionID string) (SessionMeta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	path := filepath.Join(s.root, metaDir, sessionID+".yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return SessionMeta{SessionID: sessionID}, nil
	}
	if err != nil {
		return SessionMeta{}, fmt.Errorf("store: read meta: %w", err)
	}
	var m SessionMeta
	if err := yaml.Unmarshal(data, &m); err != nil {
		return SessionMeta{}, fmt.Errorf("store: parse meta: %w", err)
	}
	m.SessionID = sessionID
	return m, nil
}

func (s *YAMLStore) PutMeta(m SessionMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.SessionID == "" {
		return errors.New("store: meta requires session id")
	}
	dir := filepath.Join(s.root, metaDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("store: create meta dir: %w", err)
	}
	return writeYAML(filepath.Join(dir, m.SessionID+".yaml"), &m)
}

// SessionByName finds a session by folder and case-insensitive name.
func (s *YAMLStore) SessionByName(folderID, name string) (Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sess := range s.sessions {
		if sess.FolderID == folderID && strings.EqualFold(sess.Name, name) {
			return *sess, true
		}
	}
	return Session{}, false
}

// FolderByName finds a folder by parent and case-insensitive name.
func (s *YAMLStore) FolderByName(parentID, name string) (Folder, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, f := range s.folders {
		if f.ParentID == parentID && strings.EqualFold(f.Name, name) {
			return *f, true
		}
	}
	return Folder{}, false
}

// FolderPath returns a display path like "Sessions/cmdb/00-JUMPHOSTS".
func (s *YAMLStore) FolderPath(folderID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	chain, err := s.folderChain(folderID)
	if err != nil {
		return ""
	}
	parts := make([]string, 0, len(chain))
	for _, f := range chain {
		parts = append(parts, f.Name)
	}
	return strings.Join(parts, "/")
}

// sessionFileName maps a session name to its on-disk file name.
func sessionFileName(name string) (string, error) {
	base := sanitizeName(name)
	if base == "" {
		return "", fmt.Errorf("store: name %q sanitizes to empty", name)
	}
	fn := base + ".yaml"
	if fn == folderFile {
		fn = "s-" + fn
	}
	return fn, nil
}

// sanitizeName restricts names to filesystem-safe characters. Spaces become
// underscores, anything outside [A-Za-z0-9._-] becomes '-', and leading or
// trailing separators are trimmed (which also neutralizes "." / ".." names).
func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('_')
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-_.")
}

// writeYAML writes v atomically: temp file in the same directory, then rename.
func writeYAML(path string, v any) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".f9-*")
	if err != nil {
		return fmt.Errorf("store: create temp: %w", err)
	}
	tmpName := tmp.Name()
	enc := yaml.NewEncoder(tmp)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("store: encode yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("store: flush yaml: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("store: close temp: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("store: chmod: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("store: rename into place: %w", err)
	}
	return nil
}
