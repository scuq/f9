package store

import (
	"errors"
	"fmt"
	"net/url"
	"time"
)

// FolderSource configures generating sessions for a folder (and its subtree)
// from an external HTTPS endpoint. Non-secret config only; the actual token /
// password / key material lives in the cred store, referenced by CredID.
type FolderSource struct {
	URL         string            `yaml:"url"`                 // https only
	Format      string            `yaml:"format"`              // f9-native | netbox | mapped
	Auth        string            `yaml:"auth"`                // none | bearer | basic | mtls
	CredID      string            `yaml:"cred_id,omitempty"`   // key into the cred store
	Header      string            `yaml:"header,omitempty"`    // custom auth header (default Authorization)
	ReconcileBy string            `yaml:"reconcile_by"`        // externalId | hostname
	FieldMap    map[string]string `yaml:"field_map,omitempty"` // for the mapped format
	Insecure    bool              `yaml:"insecure,omitempty"`  // skip TLS verification (untrusted remote cert)
	Filter      *FilterGroup      `yaml:"filter,omitempty"`    // client-side record filter (netbox)
	UpdatedAt   time.Time         `yaml:"updated_at,omitempty"`
}

var (
	validSourceFormats   = map[string]bool{"f9-native": true, "netbox": true, "mapped": true}
	validSourceAuths     = map[string]bool{"none": true, "bearer": true, "basic": true, "mtls": true}
	validSourceReconcile = map[string]bool{"externalId": true, "hostname": true}
)

func (src FolderSource) Validate() error {
	u, err := url.Parse(src.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return errors.New("store: source url must be https://host/...")
	}
	if !validSourceFormats[src.Format] {
		return fmt.Errorf("store: invalid source format %q", src.Format)
	}
	if !validSourceAuths[src.Auth] {
		return fmt.Errorf("store: invalid source auth %q", src.Auth)
	}
	if !validSourceReconcile[src.ReconcileBy] {
		return fmt.Errorf("store: invalid reconcile_by %q", src.ReconcileBy)
	}
	return nil
}

// SetFolderSource attaches an import source to a folder. It refuses the root,
// and enforces that no ancestor or descendant folder already carries a source
// (one source per subtree).
func (s *YAMLStore) SetFolderSource(folderID string, src FolderSource) error {
	if err := src.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if folderID == s.rootID {
		return errors.New("store: cannot set an import source on the root")
	}
	f, ok := s.folders[folderID]
	if !ok {
		return fmt.Errorf("store: folder %s: %w", folderID, ErrNotFound)
	}
	for pid := f.ParentID; pid != ""; {
		p, ok := s.folders[pid]
		if !ok {
			break
		}
		if p.Source != nil {
			return errors.New("store: an ancestor folder already has an import source")
		}
		pid = p.ParentID
	}
	for id, other := range s.folders {
		if id == folderID || other.Source == nil {
			continue
		}
		if s.isDescendantLocked(id, folderID) {
			return errors.New("store: a descendant folder already has an import source")
		}
	}
	dir, ok := s.folderDir[folderID]
	if !ok {
		return fmt.Errorf("store: folder dir %s: %w", folderID, ErrNotFound)
	}
	src.UpdatedAt = time.Now().UTC()
	out := *f
	cp := src
	out.Source = &cp
	out.Revision = f.Revision + 1
	out.UpdatedAt = time.Now().UTC()
	if err := writeYAML(joinDir(dir), &out); err != nil {
		return err
	}
	return s.reload()
}

// ClearFolderSource removes a folder's import source (idempotent).
func (s *YAMLStore) ClearFolderSource(folderID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.folders[folderID]
	if !ok {
		return fmt.Errorf("store: folder %s: %w", folderID, ErrNotFound)
	}
	if f.Source == nil {
		return nil
	}
	dir, ok := s.folderDir[folderID]
	if !ok {
		return fmt.Errorf("store: folder dir %s: %w", folderID, ErrNotFound)
	}
	out := *f
	out.Source = nil
	out.Revision = f.Revision + 1
	out.UpdatedAt = time.Now().UTC()
	if err := writeYAML(joinDir(dir), &out); err != nil {
		return err
	}
	return s.reload()
}

// GetFolderSource returns a copy of a folder's source, if any.
func (s *YAMLStore) GetFolderSource(folderID string) (FolderSource, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.folders[folderID]
	if !ok || f.Source == nil {
		return FolderSource{}, false
	}
	return *f.Source, true
}

// SourceFolders lists folders that carry an import source.
func (s *YAMLStore) SourceFolders() []Folder {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Folder
	for _, f := range s.folders {
		if f.Source != nil {
			out = append(out, *f)
		}
	}
	return out
}

// isDescendantLocked reports whether id sits below ancestorID in the tree.
func (s *YAMLStore) isDescendantLocked(id, ancestorID string) bool {
	f, ok := s.folders[id]
	if !ok {
		return false
	}
	for pid := f.ParentID; pid != ""; {
		if pid == ancestorID {
			return true
		}
		p, ok := s.folders[pid]
		if !ok {
			return false
		}
		pid = p.ParentID
	}
	return false
}
