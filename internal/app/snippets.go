package app

import (
	"fmt"

	"github.com/scuq/f9/internal/snippets"
)

func (a *App) SnippetFolders() []snippets.Folder { return a.snips.Folders() }
func (a *App) SnippetList() []snippets.Snippet   { return a.snips.List() }

func (a *App) SnippetGet(id string) *snippets.Snippet {
	if sn, ok := a.snips.Get(id); ok {
		return &sn
	}
	return nil
}

func (a *App) SnippetSaveFolder(f snippets.Folder) (*snippets.Folder, error) {
	saved, err := a.snips.SaveFolder(f)
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func (a *App) SnippetDeleteFolder(id string) error { return a.snips.DeleteFolder(id) }

func (a *App) SnippetSave(sn snippets.Snippet) (*snippets.Snippet, error) {
	saved, err := a.snips.SaveSnippet(sn)
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func (a *App) SnippetDelete(id string) error { return a.snips.DeleteSnippet(id) }

// SnippetRun renders a snippet against the terminal's session vars (overlaid by
// extra) and sends it with the snippet's stored paste mode.
func (a *App) SnippetRun(termID, snippetID string, extra map[string]string) error {
	a.tmu.Lock()
	t, ok := a.terms[termID]
	a.tmu.Unlock()
	if !ok {
		return fmt.Errorf("app: terminal not open")
	}
	sn, ok := a.snips.Get(snippetID)
	if !ok {
		return fmt.Errorf("app: snippet not found")
	}
	text, err := a.renderFor(t.sessionID, sn.Body, extra)
	if err != nil {
		return err
	}
	return a.sendText(termID, text, sn.DelayMs, sn.Bracketed)
}
