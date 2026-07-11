package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// OSHint is a host-keyed shadow record of a detected (or pinned) OS family.
// It survives session recreation: import reconcile joins it back onto
// sessions whose meta carries no OS yet.
type OSHint struct {
	Host       string    `yaml:"host"`
	OS         string    `yaml:"os"`
	Confidence float64   `yaml:"confidence,omitempty"`
	Pinned     bool      `yaml:"pinned,omitempty"`
	Updated    time.Time `yaml:"updated"`
}

const osHintsFile = "os-hints.yaml"

func normHost(h string) string { return strings.ToLower(strings.TrimSpace(h)) }

func (s *YAMLStore) osHintsPath() string { return filepath.Join(s.root, metaDir, osHintsFile) }

// loadOSHints reads the hint file fresh; the file is tiny and this keeps the
// method consistent with the per-call reads Meta() does.
func (s *YAMLStore) loadOSHints() map[string]OSHint {
	m := map[string]OSHint{}
	data, err := os.ReadFile(s.osHintsPath())
	if err != nil {
		return m
	}
	var list []OSHint
	if yaml.Unmarshal(data, &list) != nil {
		return m
	}
	for _, h := range list {
		m[normHost(h.Host)] = h
	}
	return m
}

// OSHint returns the stored hint for a host (normalized), if any.
func (s *YAMLStore) OSHint(host string) (OSHint, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.loadOSHints()[normHost(host)]
	return h, ok
}

// PutOSHint stores a hint for a host. A pinned hint (user-configured) is only
// replaced by another pinned write; detected hints freely update each other.
func (s *YAMLStore) PutOSHint(h OSHint) error {
	if strings.TrimSpace(h.Host) == "" || h.OS == "" {
		return errors.New("store: os hint requires host and os")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	hints := s.loadOSHints()
	key := normHost(h.Host)
	if cur, ok := hints[key]; ok && cur.Pinned && !h.Pinned {
		return nil
	}
	h.Host = key
	h.Updated = time.Now()
	hints[key] = h
	list := make([]OSHint, 0, len(hints))
	for _, v := range hints {
		list = append(list, v)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Host < list[j].Host })
	dir := filepath.Join(s.root, metaDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("store: create meta dir: %w", err)
	}
	return writeYAML(s.osHintsPath(), list)
}
