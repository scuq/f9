// Package app is the thin binding layer between the Wails frontend and the
// engine packages (store, sshx, scrollback, osdetect). It translates UI calls
// only — no business logic lives here (phase-plan 01). It deliberately imports
// no Wails packages so it compiles on every GOOS/GOARCH without cgo.
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/scuq/f9/internal/store"
)

// Version is the GUI-facing version string.
const Version = "0.1.0-phase01a"

// SessionNode is one session as the tree renders it.
type SessionNode struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	User       string `json:"user"`
	Proto      string `json:"proto"`
	DetectedOS string `json:"detectedOs"`
	OSPinned   bool   `json:"osPinned"`
}

// FolderNode is one folder with its children.
type FolderNode struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Folders  []*FolderNode `json:"folders"`
	Sessions []SessionNode `json:"sessions"`
}

type App struct {
	ctx context.Context
	st  *store.YAMLStore
}

func New() (*App, error) {
	root, err := storeRoot()
	if err != nil {
		return nil, err
	}
	st, err := store.Open(root)
	if err != nil {
		return nil, err
	}
	return &App{st: st}, nil
}

// Startup is wired to Wails OnStartup.
func (a *App) Startup(ctx context.Context) { a.ctx = ctx }

// GetVersion returns the GUI version string.
func (a *App) GetVersion() string { return Version }

// Tree reloads the store from disk and returns the full folder/session tree.
// Reload-on-call keeps hand-edited YAML visible without a watcher (a file
// watcher can replace this in a later phase).
func (a *App) Tree() (*FolderNode, error) {
	if err := a.st.LoadAll(); err != nil {
		return nil, err
	}
	nodes := map[string]*FolderNode{}
	var root *FolderNode
	for _, f := range a.st.Folders() { // sorted by (ParentID, Name)
		nodes[f.ID] = &FolderNode{ID: f.ID, Name: f.Name}
	}
	for _, f := range a.st.Folders() {
		n := nodes[f.ID]
		if f.ParentID == "" {
			root = n
			continue
		}
		if p, ok := nodes[f.ParentID]; ok {
			p.Folders = append(p.Folders, n)
		}
	}
	if root == nil {
		return nil, fmt.Errorf("app: store has no root folder")
	}
	for _, s := range a.st.Sessions() { // sorted by (FolderID, Name)
		n, ok := nodes[s.FolderID]
		if !ok {
			continue
		}
		sn := SessionNode{
			ID: s.ID, Name: s.Name, Host: s.Host, Port: s.Port,
			User: s.User, Proto: s.Proto,
		}
		if m, err := a.st.Meta(s.ID); err == nil {
			sn.DetectedOS = m.DetectedOS
			sn.OSPinned = m.OSPinned
		}
		n.Sessions = append(n.Sessions, sn)
	}
	return root, nil
}

// storeRoot mirrors the CLI's resolution (F9_STORE override, XDG default).
func storeRoot() (string, error) {
	if p := os.Getenv("F9_STORE"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("app: resolve home: %w", err)
	}
	return filepath.Join(home, ".config", "f9", "sessions"), nil
}
