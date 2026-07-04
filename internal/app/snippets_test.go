package app

import (
	"testing"

	"github.com/scuq/f9/internal/snippets"
)

func TestSnippetCRUDAndRun(t *testing.T) {
	a, id, fs := setupConnectedTerminal(t)
	if err := a.OpenTerminal("T1", id, 80, 24); err != nil {
		t.Fatal(err)
	}
	if err := a.VarsPut(VarsScopeDTO{SessionID: id}, "save_cmd", "write memory", "all"); err != nil {
		t.Fatal(err)
	}

	f, err := a.SnippetSaveFolder(snippets.Folder{Name: "cisco"})
	if err != nil {
		t.Fatal(err)
	}
	sn, err := a.SnippetSave(snippets.Snippet{FolderID: f.ID, Name: "save", Body: "{{ save_cmd }}"})
	if err != nil {
		t.Fatal(err)
	}

	if got := a.SnippetGet(sn.ID); got == nil || got.Name != "save" {
		t.Fatalf("SnippetGet = %+v", got)
	}
	if len(a.SnippetList()) != 1 || len(a.SnippetFolders()) != 1 {
		t.Fatalf("list=%d folders=%d", len(a.SnippetList()), len(a.SnippetFolders()))
	}

	if err := a.SnippetRun("T1", sn.ID, nil); err != nil {
		t.Fatal(err)
	}
	if got := fs.stdin.String(); got != "write memory\r" {
		t.Fatalf("run output = %q, want %q", got, "write memory\r")
	}

	if err := a.SnippetDeleteFolder(f.ID); err == nil {
		t.Fatal("expected non-empty folder delete rejection")
	}
	if err := a.SnippetDelete(sn.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.SnippetDeleteFolder(f.ID); err != nil {
		t.Fatal(err)
	}
}
