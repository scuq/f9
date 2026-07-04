package snippets

import "testing"

func TestFolderAndSnippetStore(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.SaveFolder(Folder{Name: "cisco"})
	if err != nil {
		t.Fatal(err)
	}
	if f.ID == "" {
		t.Fatal("folder ID not assigned")
	}
	sn, err := s.SaveSnippet(Snippet{FolderID: f.ID, Name: "save", Body: "copy run start", DelayMs: 100})
	if err != nil {
		t.Fatal(err)
	}
	if sn.ID == "" {
		t.Fatal("snippet ID not assigned")
	}

	s2, err := Open(dir) // reopen: from disk
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Get(sn.ID)
	if !ok || got.Body != "copy run start" || got.DelayMs != 100 {
		t.Fatalf("persisted snippet = %+v ok=%v", got, ok)
	}
	if len(s2.Folders()) != 1 || len(s2.List()) != 1 {
		t.Fatal("folders/list not persisted")
	}
}

func TestValidationAndDelete(t *testing.T) {
	s, _ := Open(t.TempDir())
	if _, err := s.SaveFolder(Folder{Name: ""}); err == nil {
		t.Fatal("expected empty folder name rejection")
	}
	if _, err := s.SaveSnippet(Snippet{Name: ""}); err == nil {
		t.Fatal("expected empty snippet name rejection")
	}
	if _, err := s.SaveSnippet(Snippet{Name: "x", FolderID: "nope"}); err == nil {
		t.Fatal("expected bad-folder rejection")
	}

	f, _ := s.SaveFolder(Folder{Name: "f"})
	sn, _ := s.SaveSnippet(Snippet{Name: "s", FolderID: f.ID, Body: "x"})
	if err := s.DeleteFolder(f.ID); err == nil {
		t.Fatal("expected non-empty folder rejection")
	}
	if err := s.DeleteSnippet(sn.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFolder(f.ID); err != nil {
		t.Fatal(err)
	}
	if len(s.Folders()) != 0 {
		t.Fatal("folder not deleted")
	}
}
