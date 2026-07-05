package app

import (
	"reflect"
	"testing"
	"time"

	"github.com/scuq/f9/internal/store"
)

func newTestApp(t *testing.T) (*App, *store.YAMLStore, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("F9_STORE", dir)
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutFolder(store.Folder{Name: "lab", ParentID: st.RootID()}); err != nil {
		t.Fatal(err)
	}
	lab, ok := st.FolderByName(st.RootID(), "lab")
	if !ok {
		t.Fatal("lab folder not found")
	}
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	return a, a.st, lab.ID
}

func TestTree(t *testing.T) {
	a, st, labID := newTestApp(t)
	if err := st.Put(store.Session{Name: "sw-lab-01", FolderID: labID, Host: "10.0.0.1", User: "admin"}); err != nil {
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

func TestSaveDetailProvenance(t *testing.T) {
	a, st, labID := newTestApp(t)

	// folder-level override: term type on lab
	lab, ok := st.FolderByName(st.RootID(), "lab")
	if !ok {
		t.Fatal("lab folder not found")
	}
	vt := "vt100"
	lab.Options.TermType = &vt
	if err := st.PutFolder(lab); err != nil {
		t.Fatal(err)
	}

	id, err := a.SaveSession(SessionInput{
		FolderID: labID, Name: "nx1", Host: "10.21.200.10", User: "admin",
		Options: map[string]string{"keepaliveInterval": "45s"},
	})
	if err != nil {
		t.Fatal(err)
	}
	d, err := a.SessionDetail(id)
	if err != nil {
		t.Fatal(err)
	}

	tt := d.Options["termType"]
	if tt.Value != "" || tt.Effective != "vt100" || tt.Source != "folder: Sessions/lab" {
		t.Fatalf("termType provenance = %+v", tt)
	}
	ka := d.Options["keepaliveInterval"]
	if ka.Value != "45s" || ka.Effective != "45s" || ka.Source != "session" {
		t.Fatalf("keepalive provenance = %+v", ka)
	}

	// clear the session override -> back to inherit
	if _, err := a.SaveSession(SessionInput{
		ID: id, Name: "nx1", Host: "10.21.200.10", User: "admin",
		Options: map[string]string{"keepaliveInterval": ""},
	}); err != nil {
		t.Fatal(err)
	}
	d, err = a.SessionDetail(id)
	if err != nil {
		t.Fatal(err)
	}
	if d.Options["keepaliveInterval"].Source == "session" {
		t.Fatalf("clearing option did not remove override: %+v", d.Options["keepaliveInterval"])
	}
}

func TestSaveRejectsBadOptions(t *testing.T) {
	a, _, labID := newTestApp(t)
	_, err := a.SaveSession(SessionInput{
		FolderID: labID, Name: "x", Host: "h",
		Options: map[string]string{"keepaliveInterval": "banana"},
	})
	if err == nil {
		t.Fatal("expected duration parse error")
	}
	_, err = a.SaveSession(SessionInput{
		FolderID: labID, Name: "x", Host: "h",
		Options: map[string]string{"nonsense": "1"},
	})
	if err == nil {
		t.Fatal("expected unknown-option error")
	}
}

func TestFilterBinding(t *testing.T) {
	a, st, labID := newTestApp(t)
	for _, n := range []string{"sw-lab-01", "nx-behind-jumps", "dev-behind-bastion"} {
		if err := st.Put(store.Session{Name: n, FolderID: labID, Host: "10.0.0.1"}); err != nil {
			t.Fatal(err)
		}
	}
	hits, err := a.Filter("swlab")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Name != "sw-lab-01" || hits[0].Path != "Sessions/lab" {
		t.Fatalf("Filter(swlab) = %+v", hits)
	}
}

func TestDeleteSession(t *testing.T) {
	a, _, labID := newTestApp(t)
	id, err := a.SaveSession(SessionInput{FolderID: labID, Name: "gone", Host: "h"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteSession(id); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SessionDetail(id); err == nil {
		t.Fatal("session still resolvable after delete")
	}
}

// TestProvenanceFieldGuard forces optionFields/parseOptions updates when
// store.SessionOptions grows — same pattern as the 00a overlay guard.
func TestProvenanceFieldGuard(t *testing.T) {
	const covered = 7 // 6 scalar options + JumpChain (rendered separately)
	n := reflect.TypeOf(store.SessionOptions{}).NumField()
	if n != covered {
		t.Fatalf("store.SessionOptions has %d fields, provenance covers %d — "+
			"update optionFields, parseOptions and the frontend dialog", n, covered)
	}
	_ = time.Second // keep time imported alongside future duration fields
}

func TestTreeSourceFlags(t *testing.T) {
	a, st, labID := newTestApp(t)
	if err := st.SetFolderSource(labID, store.FolderSource{URL: "https://nb/", Format: "netbox", Auth: "none", ReconcileBy: "hostname"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Put(store.Session{Name: "g1", FolderID: labID, Host: "1.1.1.1", Source: labID}); err != nil {
		t.Fatal(err)
	}
	root, err := a.Tree()
	if err != nil {
		t.Fatal(err)
	}
	var lab *FolderNode
	for _, f := range root.Folders {
		if f.ID == labID {
			lab = f
		}
	}
	if lab == nil {
		t.Fatal("lab node missing from tree")
	}
	if !lab.HasSource {
		t.Fatal("lab.HasSource should be true")
	}
	if len(lab.Sessions) != 1 || !lab.Sessions[0].Generated {
		t.Fatalf("generated flag wrong: %+v", lab.Sessions)
	}
}
