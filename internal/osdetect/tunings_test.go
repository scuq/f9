package osdetect

import (
	"path/filepath"
	"testing"
)

func TestLoadShippedTunings(t *testing.T) {
	tunings, err := LoadTunings(filepath.Join("..", "..", "configs", "os-tunings.yaml"))
	if err != nil {
		t.Fatalf("LoadTunings: %v", err)
	}
	ios, ok := tunings[FamilyIOS]
	if !ok {
		t.Fatal("ios family missing from shipped tunings")
	}
	prompt, _, _, err := ios.Compile()
	if err != nil {
		t.Fatalf("ios regexes: %v", err)
	}
	if !prompt.MatchString("sw1-core#") {
		t.Fatalf("ios prompt regex rejects sw1-core#")
	}
	if ios.Newline != "\r" {
		t.Fatalf("ios newline = %q, want \\r", ios.Newline)
	}
	for _, fam := range []Family{FamilyNXOS, FamilyPANOS, FamilyLinux, FamilyOpenBSD} {
		if _, ok := tunings[fam]; !ok {
			t.Fatalf("family %s missing from shipped tunings", fam)
		}
	}
}

func TestParseTunings(t *testing.T) {
	data := []byte("families:\n  linux:\n    prompt_regex: '[#>$]\\\\s*$'\n    error_regex: ''\n")
	m, err := ParseTunings(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m["linux"]; !ok {
		t.Fatalf("want linux family, got %v", m)
	}
}
