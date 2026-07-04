package buttonbar

import "testing"

func testChain(folderID string) []string {
	switch folderID {
	case "R":
		return []string{"R"}
	case "A":
		return []string{"R", "A"}
	case "B":
		return []string{"R", "A", "B"}
	}
	return []string{folderID}
}

func sampleBar(label string) Bar {
	return Bar{Rows: []Row{{Buttons: []Button{{Label: label, Action: Action{Kind: "send", Text: "{{ save_cmd }}"}}}}}}
}

func TestResolveInheritance(t *testing.T) {
	s, err := Open(t.TempDir(), testChain)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save("", sampleBar("global")); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("A", sampleBar("folderA")); err != nil {
		t.Fatal(err)
	}
	// B (child of A, no own bar) inherits A
	if got := s.Resolve("B"); got.Rows[0].Buttons[0].Label != "folderA" {
		t.Fatalf("Resolve(B) = %q, want folderA", got.Rows[0].Buttons[0].Label)
	}
	// A has its own
	if got := s.Resolve("A"); got.Rows[0].Buttons[0].Label != "folderA" {
		t.Fatalf("Resolve(A) = %q, want folderA", got.Rows[0].Buttons[0].Label)
	}
	// unrelated folder with no chain match -> global
	if got := s.Resolve("Z"); got.Rows[0].Buttons[0].Label != "global" {
		t.Fatalf("Resolve(Z) = %q, want global", got.Rows[0].Buttons[0].Label)
	}
}

func TestSaveGetDelete(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir, testChain)
	_ = s.Save("", sampleBar("global"))
	_ = s.Save("A", sampleBar("folderA"))

	if _, ok := s.Get("A"); !ok {
		t.Fatal("Get(A) should be defined")
	}
	if err := s.Delete("A"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("A"); ok {
		t.Fatal("Get(A) should be gone after delete")
	}
	if got := s.Resolve("A"); got.Rows[0].Buttons[0].Label != "global" {
		t.Fatalf("Resolve(A) after delete = %q, want global", got.Rows[0].Buttons[0].Label)
	}

	// reopen: persisted
	s2, _ := Open(dir, testChain)
	if _, ok := s2.Get(""); !ok {
		t.Fatal("global bar should persist")
	}
}

func TestImportExport(t *testing.T) {
	s, _ := Open(t.TempDir(), testChain)
	_ = s.Save("A", sampleBar("folderA"))
	yamlText, err := s.Export("A")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Import("B", yamlText); err != nil {
		t.Fatal(err)
	}
	if got := s.Resolve("B"); got.Rows[0].Buttons[0].Label != "folderA" {
		t.Fatalf("imported bar label = %q, want folderA", got.Rows[0].Buttons[0].Label)
	}
}

func TestSaveRejectsInvalid(t *testing.T) {
	s, _ := Open(t.TempDir(), nil)
	bad := Bar{Rows: []Row{{Buttons: []Button{{Label: "", Action: Action{Kind: "send"}}}}}}
	if err := s.Save("A", bad); err == nil {
		t.Fatal("expected empty-label rejection")
	}
	badKind := Bar{Rows: []Row{{Buttons: []Button{{Label: "x", Action: Action{Kind: "nope"}}}}}}
	if err := s.Save("A", badKind); err == nil {
		t.Fatal("expected invalid-kind rejection")
	}
}

func TestResolveFolderNoGlobalFallback(t *testing.T) {
	s, _ := Open(t.TempDir(), testChain)
	_ = s.Save("", sampleBar("global"))
	if got := s.ResolveFolder("A"); len(got.Rows) != 0 {
		t.Fatalf("ResolveFolder(A) should be empty, got %+v", got)
	}
	_ = s.Save("A", sampleBar("folderA"))
	if got := s.ResolveFolder("B"); got.Rows[0].Buttons[0].Label != "folderA" {
		t.Fatalf("ResolveFolder(B) = %q, want folderA", got.Rows[0].Buttons[0].Label)
	}
}

func labels(b Bar) []string {
	var out []string
	for _, r := range b.Rows {
		for _, btn := range r.Buttons {
			out = append(out, btn.Label)
		}
	}
	return out
}

func TestFilterOS(t *testing.T) {
	bar := Bar{Rows: []Row{{Buttons: []Button{
		{Label: "always", Action: Action{Kind: "send"}},
		{Label: "ios-only", OS: "ios", Action: Action{Kind: "send"}},
		{Label: "undetected", OS: "unknown", Action: Action{Kind: "send"}},
	}}}}
	if got := labels(bar.FilterOS("ios")); len(got) != 2 || got[0] != "always" || got[1] != "ios-only" {
		t.Fatalf("ios filter = %v", got)
	}
	if got := labels(bar.FilterOS("linux")); len(got) != 1 || got[0] != "always" {
		t.Fatalf("linux filter = %v", got)
	}
	if got := labels(bar.FilterOS("")); len(got) != 2 || got[1] != "undetected" {
		t.Fatalf("undetected filter = %v", got)
	}
}

func TestSaveRejectsBadOS(t *testing.T) {
	s, _ := Open(t.TempDir(), nil)
	bad := Bar{Rows: []Row{{Buttons: []Button{{Label: "x", OS: "bogus", Action: Action{Kind: "send"}}}}}}
	if err := s.Save("A", bad); err == nil {
		t.Fatal("expected invalid-os rejection")
	}
}
