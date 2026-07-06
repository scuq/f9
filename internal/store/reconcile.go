package store

import "fmt"

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
	Attrs      map[string]string // filterable attributes (status, role, manufacturer, model, hostname, tenant, site)
}

// ReconcileResult reports what a reconcile changed.
type ReconcileResult struct {
	Added   int
	Updated int
	Removed int
}

// ReconcileFolderSessions makes the generated sessions under folderID match
// records, keyed by reconcileBy ("hostname" or "externalId"). Only sessions
// owned by this source (Source == folderID) are touched; hand-made sessions in
// the folder are left alone, and generated sessions absent from records are
// removed.
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
		if sess.FolderID == folderID && sess.Source == folderID {
			existing[keyOf(sess.ExternalID, sess.Host)] = sess
		}
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
		k := keyOf(r.ExternalID, r.Host)
		seen[k] = true
		if cur, ok := existing[k]; ok {
			cur.Name = r.Name
			cur.Host = r.Host
			cur.Port = port
			cur.User = r.User
			cur.Proto = proto
			cur.Tags = r.Tags
			cur.ExternalID = r.ExternalID
			cur.Source = folderID
			if err := s.Put(cur); err != nil {
				return res, fmt.Errorf("store: reconcile update %q: %w", r.Name, err)
			}
			res.Updated++
			continue
		}
		ns := Session{
			Name:       r.Name,
			FolderID:   folderID,
			Host:       r.Host,
			Port:       port,
			User:       r.User,
			Proto:      proto,
			Tags:       r.Tags,
			Source:     folderID,
			ExternalID: r.ExternalID,
		}
		if err := s.Put(ns); err != nil {
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
	return res, nil
}

func (s *YAMLStore) folderExists(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.folders[id]
	return ok
}
