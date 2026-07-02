package store

import (
	"errors"
	"os"
	"testing"
	"time"
)

func openTemp(t *testing.T) *YAMLStore {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

func TestStoreInitCreatesRoot(t *testing.T) {
	s := openTemp(t)
	if s.RootID() == "" {
		t.Fatal("root folder id empty")
	}
	if got := len(s.Folders()); got != 1 {
		t.Fatalf("folders = %d, want 1 (root)", got)
	}
}

func TestPutAndRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.PutFolder(Folder{Name: "cmdb", ParentID: s.RootID()}); err != nil {
		t.Fatalf("PutFolder: %v", err)
	}
	cmdb, ok := s.FolderByName(s.RootID(), "cmdb")
	if !ok {
		t.Fatal("cmdb folder not found")
	}

	sess := Session{Name: "SU00NJU100 10.21.194.1", FolderID: cmdb.ID, Host: "10.21.194.1", Port: 22, User: "ste9933"}
	if err := s.Put(sess); err != nil {
		t.Fatalf("Put: %v", err)
	}

	s2, err := Open(dir) // reopen from disk
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, ok := s2.SessionByName(cmdb.ID, "SU00NJU100 10.21.194.1")
	if !ok {
		t.Fatal("session not found after reload")
	}
	if got.Host != "10.21.194.1" || got.User != "ste9933" || got.Port != 22 {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if got.Revision != 1 {
		t.Fatalf("revision = %d, want 1", got.Revision)
	}
	if got.Proto != "ssh" {
		t.Fatalf("proto default = %q, want ssh", got.Proto)
	}
	if got.ID == "" {
		t.Fatal("id not assigned")
	}
}

func TestRevisionBumpAndRename(t *testing.T) {
	s := openTemp(t)
	if err := s.Put(Session{Name: "core-01", FolderID: s.RootID(), Host: "10.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.SessionByName(s.RootID(), "core-01")

	got.Name = "core-01-renamed"
	if err := s.Put(got); err != nil {
		t.Fatalf("rename Put: %v", err)
	}
	got2, ok := s.SessionByName(s.RootID(), "core-01-renamed")
	if !ok {
		t.Fatal("renamed session missing")
	}
	if got2.Revision != 2 {
		t.Fatalf("revision = %d, want 2", got2.Revision)
	}
	if _, ok := s.SessionByName(s.RootID(), "core-01"); ok {
		t.Fatal("old name still resolves")
	}
	entries, err := os.ReadDir(s.folderDir[s.RootID()])
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == "core-01.yaml" {
			t.Fatal("old session file still on disk")
		}
	}
}

func TestNameCollision(t *testing.T) {
	s := openTemp(t)
	if err := s.Put(Session{Name: "dup", FolderID: s.RootID(), Host: "h1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(Session{Name: "DUP", FolderID: s.RootID(), Host: "h2"}); err == nil {
		t.Fatal("expected case-insensitive name collision error")
	}
}

func TestResolveThroughStore(t *testing.T) {
	s := openTemp(t)

	root := Folder{ID: s.RootID()} // root options = "Default Session"
	root.Options.TermType = strp("xterm-256color")
	root.Options.KeepaliveInterval = durp(30 * time.Second)
	if err := s.PutFolder(root); err != nil {
		t.Fatal(err)
	}

	f := Folder{Name: "dc", ParentID: s.RootID()}
	f.Options.TermType = strp("vt100")
	if err := s.PutFolder(f); err != nil {
		t.Fatal(err)
	}
	dc, ok := s.FolderByName(s.RootID(), "dc")
	if !ok {
		t.Fatal("dc folder not found")
	}

	sess := Session{Name: "nx1", FolderID: dc.ID, Host: "10.1.1.1"}
	sess.Options.ScrollbackLines = intp(5000000)
	if err := s.Put(sess); err != nil {
		t.Fatal(err)
	}
	nx1, ok := s.SessionByName(dc.ID, "nx1")
	if !ok {
		t.Fatal("nx1 not found")
	}

	_, eff, err := s.Resolve(nx1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if eff.TermType == nil || *eff.TermType != "vt100" {
		t.Fatalf("TermType = %v, want folder override vt100", eff.TermType)
	}
	if eff.KeepaliveInterval == nil || *eff.KeepaliveInterval != 30*time.Second {
		t.Fatal("keepalive not inherited from root defaults")
	}
	if eff.ScrollbackLines == nil || *eff.ScrollbackLines != 5000000 {
		t.Fatal("session-level option lost")
	}
}

func TestDeleteRules(t *testing.T) {
	s := openTemp(t)
	if err := s.PutFolder(Folder{Name: "lab", ParentID: s.RootID()}); err != nil {
		t.Fatal(err)
	}
	lab, _ := s.FolderByName(s.RootID(), "lab")
	if err := s.Put(Session{Name: "x", FolderID: lab.ID, Host: "h"}); err != nil {
		t.Fatal(err)
	}
	x, _ := s.SessionByName(lab.ID, "x")

	if err := s.Delete(lab.ID); err == nil {
		t.Fatal("expected non-empty folder delete to fail")
	}
	if err := s.Delete(x.ID); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if err := s.Delete(lab.ID); err != nil {
		t.Fatalf("delete empty folder: %v", err)
	}
	if err := s.Delete(s.RootID()); err == nil {
		t.Fatal("expected root delete to fail")
	}
	if err := s.Delete("01NOPE"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestMetaRoundtrip(t *testing.T) {
	s := openTemp(t)
	if err := s.Put(Session{Name: "m1", FolderID: s.RootID(), Host: "h"}); err != nil {
		t.Fatal(err)
	}
	m1, _ := s.SessionByName(s.RootID(), "m1")

	m, err := s.Meta(m1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if m.DetectedOS != "" {
		t.Fatal("expected zero meta before first PutMeta")
	}

	m.DetectedOS = "nxos"
	m.OSConfidence = 0.93
	m.LastConnect = time.Now().UTC()
	if err := s.PutMeta(m); err != nil {
		t.Fatal(err)
	}

	m2, err := s.Meta(m1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if m2.DetectedOS != "nxos" || m2.OSConfidence != 0.93 {
		t.Fatalf("meta roundtrip mismatch: %+v", m2)
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"SU00NJU100 10.21.194.1": "SU00NJU100_10.21.194.1",
		"föö/bar":                "f---bar",
		"..":                     "",
		"a b c":                  "a_b_c",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Fatalf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}
