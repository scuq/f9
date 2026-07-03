// Package store is the session/folder store: YAML-on-disk, fully indexed in
// memory, with SecureCRT-style option inheritance (defaults -> folder chain ->
// session). See docs/phase-plan.md 00a and ADR-0006.
package store

import "time"

// SessionOptions: every field is a pointer; nil means "inherit".
type SessionOptions struct {
	TermType          *string        `yaml:"term_type,omitempty"`
	KeepaliveInterval *time.Duration `yaml:"keepalive_interval,omitempty"`
	Reconnect         *string        `yaml:"reconnect,omitempty"` // off|prompt|auto
	ThemeRef          *string        `yaml:"theme,omitempty"`
	JumpChain         []JumpHop      `yaml:"jump_chain,omitempty"` // non-nil overrides
	ScrollbackLines   *int           `yaml:"scrollback_lines,omitempty"`
	AuditScope        *string        `yaml:"audit_scope,omitempty"` // off|events|events+input|full-io
}

// JumpHop is one hop of a jump chain. Mode "proxyjump" (TCP forward) or
// "shell-hop" (interactive ssh from the hop; for bastions without forwarding).
type JumpHop struct {
	Host         string `yaml:"host"`
	Port         int    `yaml:"port,omitempty"`
	User         string `yaml:"user,omitempty"`
	Mode         string `yaml:"mode"` // proxyjump|shell-hop
	UserOverride string `yaml:"user_override,omitempty"`
}

type Folder struct {
	ID        string         `yaml:"id"` // ULID
	Name      string         `yaml:"name"`
	ParentID  string         `yaml:"parent_id,omitempty"`
	Options   SessionOptions `yaml:"options,omitempty"`
	Revision  uint64         `yaml:"revision"`
	UpdatedAt time.Time      `yaml:"updated_at"`
}

type Session struct {
	ID        string         `yaml:"id"` // ULID
	Name      string         `yaml:"name"`
	FolderID  string         `yaml:"folder_id"`
	Host      string         `yaml:"host"`
	Port      int            `yaml:"port"`
	User      string         `yaml:"user,omitempty"`
	Proto     string         `yaml:"proto"` // ssh (serial/telnet later)
	Tags      []string       `yaml:"tags,omitempty"`
	Options   SessionOptions `yaml:"options,omitempty"`
	Revision  uint64         `yaml:"revision"`
	UpdatedAt time.Time      `yaml:"updated_at"`
}

// SessionMeta is machine-written sidecar state (never user-edited).
type SessionMeta struct {
	SessionID    string    `yaml:"session_id"`
	DetectedOS   string    `yaml:"detected_os,omitempty"` // osdetect.Family
	OSConfidence float64   `yaml:"os_confidence,omitempty"`
	OSPinned     bool      `yaml:"os_pinned,omitempty"`
	LastConnect  time.Time `yaml:"last_connect,omitempty"`
	Pinned       bool      `yaml:"pinned,omitempty"`
}

// Store is the persistence boundary; the YAML-tree implementation is phase 00a,
// and it must stay narrow enough that SQLite can replace it.
type Store interface {
	LoadAll() error
	Folders() []Folder
	Sessions() []Session
	Resolve(sessionID string) (Session, SessionOptions, error) // effective options
	Put(s Session) error                                       // bumps Revision, UpdatedAt
	PutFolder(f Folder) error
	Delete(id string) error
	Meta(sessionID string) (SessionMeta, error)
	PutMeta(m SessionMeta) error
}
