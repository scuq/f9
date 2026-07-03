package app

import (
	"testing"

	"github.com/scuq/f9/internal/store"
)

func TestTree(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("F9_STORE", dir)

	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutFolder(store.Folder{Name: "lab", ParentID: st.RootID()}); err != nil {
		t.Fatal(err)
	}
	lab, _ := st.FolderByName(st.RootID(), "lab")
	if err := st.Put(store.Session{Name: "sw-lab-01", FolderID: lab.ID, Host: "10.0.0.1", User: "admin"}); err != nil {
		t.Fatal(err)
	}

	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	root, err := a.Tree()
	if err != nil {
		t.Fatal(err)
	}
	if len(root.Folders) != 1 || root.Folders[0].Name != "lab" {
		t.Fatalf("root folders = %+v, want [lab]", root.Folders)
	}
	sess := root.Folders[0].Sessions
	if len(sess) != 1 || sess[0].Name != "sw-lab-01" || sess[0].Host != "10.0.0.1" {
		t.Fatalf("lab sessions = %+v", sess)
	}
}
