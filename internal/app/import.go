package app

import (
	"context"
	"fmt"
	"time"

	"github.com/scuq/f9/internal/sessionimport"
	"github.com/scuq/f9/internal/store"
)

// CredState reports the credential store status for the UI's unlock flow.
type CredState struct {
	Initialized bool `json:"initialized"`
	Locked      bool `json:"locked"`
}

// CredStatus reports whether a passphrase exists and whether the store is locked.
func (a *App) CredStatus() CredState {
	return CredState{Initialized: a.creds.Initialized(), Locked: a.creds.Locked()}
}

// CredSetPassphrase performs first-time passphrase setup (and unlocks).
func (a *App) CredSetPassphrase(pass string) error { return a.creds.SetPassphrase(pass) }

// CredUnlock unlocks the credential store for this app run.
func (a *App) CredUnlock(pass string) error { return a.creds.Unlock(pass) }

// SourceDTO is the non-secret folder-source config exchanged with the UI.
type SourceDTO struct {
	URL         string            `json:"url"`
	Format      string            `json:"format"`
	Auth        string            `json:"auth"`
	Header      string            `json:"header"`
	ReconcileBy string            `json:"reconcileBy"`
	Insecure    bool              `json:"insecure"`
	FieldMap    map[string]string `json:"fieldMap"`
	HasSecret   bool              `json:"hasSecret"`
}

func credIDFor(folderID string) string { return "src:" + folderID }

func (dto SourceDTO) toSource(folderID string) store.FolderSource {
	return store.FolderSource{
		URL: dto.URL, Format: dto.Format, Auth: dto.Auth, Header: dto.Header,
		ReconcileBy: dto.ReconcileBy, Insecure: dto.Insecure, FieldMap: dto.FieldMap,
		CredID: credIDFor(folderID),
	}
}

// FolderSourceGet returns the folder's source config (no secret), or nil.
func (a *App) FolderSourceGet(folderID string) *SourceDTO {
	src, ok := a.st.GetFolderSource(folderID)
	if !ok {
		return nil
	}
	return &SourceDTO{
		URL: src.URL, Format: src.Format, Auth: src.Auth, Header: src.Header,
		ReconcileBy: src.ReconcileBy, Insecure: src.Insecure, FieldMap: src.FieldMap,
		HasSecret: a.creds.Has(src.CredID),
	}
}

// FolderSourceSet validates and saves a source. A supplied secret is written to
// the (unlocked) cred store; an empty secret keeps any existing one.
func (a *App) FolderSourceSet(folderID string, dto SourceDTO, secret string) error {
	src := dto.toSource(folderID)
	if err := src.Validate(); err != nil {
		return err
	}
	if dto.Auth != "none" {
		if secret != "" {
			if !a.creds.Initialized() {
				return fmt.Errorf("app: set a credential passphrase first")
			}
			if a.creds.Locked() {
				return fmt.Errorf("app: unlock the credential store first")
			}
			if err := a.creds.Put(src.CredID, secret); err != nil {
				return err
			}
		} else if !a.creds.Has(src.CredID) {
			return fmt.Errorf("app: %s auth requires a credential", dto.Auth)
		}
	}
	return a.st.SetFolderSource(folderID, src)
}

// FolderSourceClear removes the source and its stored secret.
func (a *App) FolderSourceClear(folderID string) error {
	if src, ok := a.st.GetFolderSource(folderID); ok {
		_ = a.creds.Delete(src.CredID)
	}
	return a.st.ClearFolderSource(folderID)
}

// TestResult reports a dry connectivity/auth/format check.
type TestResult struct {
	OK     bool     `json:"ok"`
	Count  int      `json:"count"`
	Sample []string `json:"sample"`
	Error  string   `json:"error"`
}

// FolderSourceTest fetches + decodes with the given config and secret without
// saving or reconciling. If secret is empty, a stored secret is used (unlocked).
func (a *App) FolderSourceTest(folderID string, dto SourceDTO, secret string) TestResult {
	src := dto.toSource(folderID)
	if err := src.Validate(); err != nil {
		return TestResult{Error: err.Error()}
	}
	sec, err := a.resolveSecret(src, secret)
	if err != nil {
		return TestResult{Error: err.Error()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	body, err := sessionimport.Fetch(ctx, src, sec)
	if err != nil {
		return TestResult{Error: err.Error()}
	}
	recs, err := sessionimport.Decode(src.Format, src.FieldMap, body)
	if err != nil {
		return TestResult{Error: err.Error()}
	}
	sample := make([]string, 0, 5)
	for i, r := range recs {
		if i >= 5 {
			break
		}
		name := r.Name
		if name == "" {
			name = r.Host
		}
		sample = append(sample, name)
	}
	return TestResult{OK: true, Count: len(recs), Sample: sample}
}

// RefreshResult reports a reconcile outcome.
type RefreshResult struct {
	Added   int    `json:"added"`
	Updated int    `json:"updated"`
	Removed int    `json:"removed"`
	Error   string `json:"error"`
}

// FolderSourceRefresh fetches + decodes + reconciles the folder's generated
// sessions.
func (a *App) FolderSourceRefresh(folderID string) RefreshResult {
	src, ok := a.st.GetFolderSource(folderID)
	if !ok {
		return RefreshResult{Error: "no import source on this folder"}
	}
	sec, err := a.resolveSecret(src, "")
	if err != nil {
		return RefreshResult{Error: err.Error()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	body, err := sessionimport.Fetch(ctx, src, sec)
	if err != nil {
		return RefreshResult{Error: err.Error()}
	}
	recs, err := sessionimport.Decode(src.Format, src.FieldMap, body)
	if err != nil {
		return RefreshResult{Error: err.Error()}
	}
	res, err := a.st.ReconcileFolderSessions(folderID, recs, src.ReconcileBy)
	if err != nil {
		return RefreshResult{Error: err.Error()}
	}
	return RefreshResult{Added: res.Added, Updated: res.Updated, Removed: res.Removed}
}

// resolveSecret returns the credential for a source: the override, the stored
// secret (requires unlock), or "" for auth=none.
func (a *App) resolveSecret(src store.FolderSource, override string) (string, error) {
	if src.Auth == "none" {
		return "", nil
	}
	if override != "" {
		return override, nil
	}
	if !a.creds.Initialized() {
		return "", fmt.Errorf("app: no stored credential; set a passphrase and enter the credential")
	}
	if a.creds.Locked() {
		return "", fmt.Errorf("app: unlock the credential store first")
	}
	sec, err := a.creds.Get(src.CredID)
	if err != nil {
		return "", fmt.Errorf("app: credential: %w", err)
	}
	return sec, nil
}
