package store

import "testing"

func mkFolder(t *testing.T, s *YAMLStore, name, parent string) Folder {
	t.Helper()
	if err := s.PutFolder(Folder{Name: name, ParentID: parent}); err != nil {
		t.Fatal(err)
	}
	f, ok := s.FolderByName(parent, name)
	if !ok {
		t.Fatalf("folder %q not found after create", name)
	}
	return f
}

func sampleSource() FolderSource {
	return FolderSource{
		URL:         "https://nb.example.at/api/dcim/devices/",
		Format:      "netbox",
		Auth:        "bearer",
		CredID:      "c1",
		ReconcileBy: "hostname",
	}
}

func TestSetGetClearSource(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := mkFolder(t, s, "lab", s.RootID())
	if err := s.SetFolderSource(f.ID, sampleSource()); err != nil {
		t.Fatal(err)
	}
	got, ok := s.GetFolderSource(f.ID)
	if !ok || got.Format != "netbox" || got.URL == "" {
		t.Fatalf("get source = %+v ok=%v", got, ok)
	}
	if len(s.SourceFolders()) != 1 {
		t.Fatalf("SourceFolders = %d", len(s.SourceFolders()))
	}
	if err := s.ClearFolderSource(f.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GetFolderSource(f.ID); ok {
		t.Fatal("source should be cleared")
	}
}

func TestSourceValidation(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := mkFolder(t, s, "lab", s.RootID())
	bad := []FolderSource{
		{URL: "http://x/", Format: "netbox", Auth: "bearer", ReconcileBy: "hostname"},
		{URL: "https://x/", Format: "bogus", Auth: "bearer", ReconcileBy: "hostname"},
		{URL: "https://x/", Format: "netbox", Auth: "bogus", ReconcileBy: "hostname"},
		{URL: "https://x/", Format: "netbox", Auth: "bearer", ReconcileBy: "bogus"},
	}
	for i, b := range bad {
		if err := s.SetFolderSource(f.ID, b); err == nil {
			t.Fatalf("case %d should be rejected", i)
		}
	}
}

func TestSourceRootRejected(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetFolderSource(s.RootID(), sampleSource()); err == nil {
		t.Fatal("root source should be rejected")
	}
}

func TestSourceSubtreeUniqueness(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent := mkFolder(t, s, "parent", s.RootID())
	child := mkFolder(t, s, "child", parent.ID)

	if err := s.SetFolderSource(parent.ID, sampleSource()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFolderSource(child.ID, sampleSource()); err == nil {
		t.Fatal("child under a sourced parent should be rejected")
	}
	if err := s.ClearFolderSource(parent.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFolderSource(child.ID, sampleSource()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFolderSource(parent.ID, sampleSource()); err == nil {
		t.Fatal("parent above a sourced child should be rejected")
	}
}
