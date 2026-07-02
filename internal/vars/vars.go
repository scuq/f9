// Package vars is the scoped variable store (global -> folder -> session,
// same resolution semantics as session options). Secrets are rejected by key
// naming policy (ADR-0005); use the agent or interactive prompts instead.
// Consumers: snippet templating (pongo2), button-bar send-strings, Lua (read-only).
package vars

type Scope struct {
	FolderID  string // "" = global
	SessionID string // "" = folder/global level
}

type Store interface {
	Get(s Scope, key string) (string, bool)
	List(s Scope) map[string]string // fully resolved view
	Put(s Scope, key, value string) error
	Delete(s Scope, key string) error
}
