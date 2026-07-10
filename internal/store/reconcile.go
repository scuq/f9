package store

import (
	"errors"
	"fmt"
	"strings"
)

// ImportRecord is a decoded, source-shaped session, produced by
// internal/sessionimport and consumed by ReconcileFolderSessions.
type ImportRecord struct {
	ExternalID string
	Name       string
	Host       string
	Port       int
	User       string
	Proto      string
	Tags       []string
	Attrs      map[string]string      // filterable attributes (status, role, manufacturer, model, hostname, tenant, site)
	Raw        map[string]interface{} // full decoded source object (exposed to map scripts as r.raw)
	Folder     string                 // nested folder path under the source folder (e.g. "site/role"); "" = the source folder itself
}

// ReconcileResult reports what a reconcile changed.
type ReconcileResult struct {
	Added   int
	Updated int
	Removed int
	Skipped int // records dropped due to a name collision (import continued)
}

// maxFolderDepth bounds the nested folder paths a record may create.
const maxFolderDepth = 8

// ReconcileFolderSessions makes the generated sessions owned by folderID
// (Source == folderID, anywhere in its subtree) match records, keyed by
// reconcileBy ("hostname" or "externalId"). Each record's Folder path is
// created under folderID as needed (created folders carry SourceOwner) and the
// session is placed — or moved — into its leaf. Hand-made sessions are left
// alone; generated sessions absent from records are removed, and empty
// auto-created folders are pruned.
func (s *YAMLStore) ReconcileFolderSessions(folderID string, records []ImportRecord, reconcileBy string) (ReconcileResult, error) {
	var res ReconcileResult
	if !s.folderExists(folderID) {
		return res, fmt.Errorf("store: folder %s: %w", folderID, ErrNotFound)
	}
	keyOf := func(extID, host string) string {
		if reconcileBy == "externalId" {
			return "e:" + extID
		}
		return "h:" + host
	}

	existing := map[string]Session{}
	for _, sess := range s.Sessions() {
		if sess.Source == folderID {
			existing[keyOf(sess.ExternalID, sess.Host)] = sess
		}
	}

	leafCache := map[string]string{"": folderID}
	leafFor := func(path string) (string, error) {
		if id, ok := leafCache[path]; ok {
			return id, nil
		}
		id, err := s.ensureFolderPath(folderID, path)
		if err != nil {
			return "", err
		}
		leafCache[path] = id
		return id, nil
	}

	seen := map[string]bool{}
	for _, r := range records {
		port := r.Port
		if port == 0 {
			port = 22
		}
		proto := r.Proto
		if proto == "" {
			proto = "ssh"
		}
		leaf, err := leafFor(strings.Trim(r.Folder, "/"))
		if err != nil {
			return res, fmt.Errorf("store: reconcile folder %q: %w", r.Folder, err)
		}
		k := keyOf(r.ExternalID, r.Host)
		seen[k] = true
		if cur, ok := existing[k]; ok {
			cur.Name = r.Name
			cur.FolderID = leaf
			cur.Host = r.Host
			cur.Port = port
			cur.User = r.User
			cur.Proto = proto
			cur.Tags = r.Tags
			cur.ExternalID = r.ExternalID
			cur.Source = folderID
			if err := s.Put(cur); err != nil {
				if errors.Is(err, ErrDuplicateName) {
					res.Skipped++
					continue
				}
				return res, fmt.Errorf("store: reconcile update %q: %w", r.Name, err)
			}
			res.Updated++
			continue
		}
		ns := Session{
			Name:       r.Name,
			FolderID:   leaf,
			Host:       r.Host,
			Port:       port,
			User:       r.User,
			Proto:      proto,
			Tags:       r.Tags,
			Source:     folderID,
			ExternalID: r.ExternalID,
		}
		if err := s.Put(ns); err != nil {
			if errors.Is(err, ErrDuplicateName) {
				res.Skipped++
				continue
			}
			return res, fmt.Errorf("store: reconcile add %q: %w", r.Name, err)
		}
		res.Added++
	}

	for k, sess := range existing {
		if !seen[k] {
			if err := s.Delete(sess.ID); err != nil {
				return res, fmt.Errorf("store: reconcile remove %q: %w", sess.Name, err)
			}
			res.Removed++
		}
	}

	if err := s.pruneGeneratedFolders(folderID); err != nil {
		return res, err
	}
	return res, nil
}

// ensureFolderPath creates (or reuses) the nested folder path under sourceID
// and returns the leaf folder's ID. Folders it creates carry SourceOwner =
// sourceID so pruning can distinguish them from hand-made folders.
func (s *YAMLStore) ensureFolderPath(sourceID, path string) (string, error) {
	if path == "" {
		return sourceID, nil
	}
	var parts []string
	for _, p := range strings.Split(path, "/") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == "." || p == ".." {
			return "", fmt.Errorf("store: invalid folder path component %q", p)
		}
		parts = append(parts, p)
	}
	if len(parts) > maxFolderDepth {
		return "", fmt.Errorf("store: folder path deeper than %d: %q", maxFolderDepth, path)
	}
	parent := sourceID
	for _, name := range parts {
		if f, ok := s.FolderByName(parent, name); ok {
			parent = f.ID
			continue
		}
		if err := s.PutFolder(Folder{Name: name, ParentID: parent, SourceOwner: sourceID}); err != nil {
			return "", err
		}
		f, ok := s.FolderByName(parent, name)
		if !ok {
			return "", fmt.Errorf("store: folder %q missing after create", name)
		}
		parent = f.ID
	}
	return parent, nil
}

// pruneGeneratedFolders deletes empty folders auto-created for sourceID,
// repeating passes so emptied parents are pruned too.
func (s *YAMLStore) pruneGeneratedFolders(sourceID string) error {
	for pass := 0; pass <= maxFolderDepth; pass++ {
		hasChild := map[string]bool{}
		for _, f := range s.Folders() {
			hasChild[f.ParentID] = true
		}
		hasSess := map[string]bool{}
		for _, sess := range s.Sessions() {
			hasSess[sess.FolderID] = true
		}
		deleted := false
		for _, f := range s.Folders() {
			if f.SourceOwner != sourceID || hasChild[f.ID] || hasSess[f.ID] {
				continue
			}
			if err := s.Delete(f.ID); err != nil {
				return fmt.Errorf("store: prune folder %q: %w", f.Name, err)
			}
			deleted = true
		}
		if !deleted {
			return nil
		}
	}
	return nil
}

func (s *YAMLStore) folderExists(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.folders[id]
	return ok
}
