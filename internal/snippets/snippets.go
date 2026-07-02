// Package snippets: tree-organized command snippets with jinja2-style
// templating (pongo2 in phase 05), variable resolution from the vars store,
// prompt-for-unresolved at paste time, and paste modes including per-line
// delay for device config pastes.
package snippets

type Snippet struct {
	ID       string
	FolderID string
	Name     string
	Body     string // pongo2 template; multiline
}

type PasteMode struct {
	LineDelayMs int  // e.g. 100 for Cisco config paste
	Bracketed   bool // bracketed-paste for shells that support it
}
