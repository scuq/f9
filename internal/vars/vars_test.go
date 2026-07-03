package vars

import (
	"os"
	"path/filepath"
	"testing"
)

// testChain models a tree: root "R" -> "A" -> "B".
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

func TestScopedResolution(t *testing.T) {
	s, err := Open(t.TempDir(), testChain)
	if err != nil {
		t.Fatal(err)
	}
	must := func(e error) { t.Helper(); if e != nil { t.Fatal(e) } }

	must(s.Put(Scope{}, "region", "eu", "all"))
	must(s.Put(Scope{}, "domain", "kages.local", "all"))
	must(s.Put(Scope{FolderID: "A"}, "region", "at", "all"))
	must(s.Put(Scope{FolderID: "B"}, "vlan_id", "222", "all"))
	must(s.Put(Scope{FolderID: "B", SessionID: "S1"}, "vlan_id", "999", "all"))

	if v, _ := s.Get(Scope{}, "region", ""); v != "eu" {
		t.Fatalf("global region = %q", v)
	}
	fa := s.List(Scope{FolderID: "A"}, "")
	if fa["region"] != "at" || fa["domain"] != "kages.local" {
		t.Fatalf("folder A view = %+v", fa)
	}
	sv := s.List(Scope{FolderID: "B", SessionID: "S1"}, "")
	if sv["region"] != "at" || sv["domain"] != "kages.local" || sv["vlan_id"] != "999" {
		t.Fatalf("session S1 view = %+v", sv)
	}
}

func TestScalarIsAll(t *testing.T) {
	s, _ := Open(t.TempDir(), nil)
	_ = s.Put(Scope{}, "domain", "kages.local", "all")
	for _, fam := range []string{"", "ios", "linux"} {
		if v, _ := s.Get(Scope{}, "domain", fam); v != "kages.local" {
			t.Fatalf("family %q: domain = %q", fam, v)
		}
	}
}

func TestFamilyBeatsAll(t *testing.T) {
	s, _ := Open(t.TempDir(), nil)
	_ = s.Put(Scope{}, "save_cmd", "write memory", "all")
	_ = s.Put(Scope{}, "save_cmd", "copy run start", "nxos")

	if v, _ := s.Get(Scope{}, "save_cmd", "nxos"); v != "copy run start" {
		t.Fatalf("nxos save_cmd = %q, want copy run start", v)
	}
	if v, _ := s.Get(Scope{}, "save_cmd", "ios"); v != "write memory" {
		t.Fatalf("ios save_cmd (all fallback) = %q, want write memory", v)
	}
	if v, _ := s.Get(Scope{}, "save_cmd", ""); v != "write memory" {
		t.Fatalf("undetected save_cmd (all fallback) = %q, want write memory", v)
	}
}

func TestUnknownOnly(t *testing.T) {
	s, _ := Open(t.TempDir(), nil)
	_ = s.Put(Scope{}, "banner", "detecting", "unknown")
	if v, _ := s.Get(Scope{}, "banner", ""); v != "detecting" {
		t.Fatalf("undetected banner = %q, want detecting", v)
	}
	if _, ok := s.Get(Scope{}, "banner", "ios"); ok {
		t.Fatal("banner should be absent for detected ios (no ios, no all)")
	}
}

func TestSecretAndKeyValidation(t *testing.T) {
	s, _ := Open(t.TempDir(), nil)
	if err := s.Put(Scope{}, "api_token", "x", "all"); err == nil {
		t.Fatal("expected secret rejection")
	}
	if err := s.Put(Scope{}, "dash-key", "x", "all"); err == nil {
		t.Fatal("expected invalid-key rejection")
	}
	if err := s.Put(Scope{}, "save_cmd", "x", "bogusos"); err == nil {
		t.Fatal("expected invalid-OS rejection")
	}
	if err := s.Put(Scope{}, "vlan_id", "10", ""); err != nil {
		t.Fatalf("valid put with empty os: %v", err)
	}
}

func TestDeleteSelectorAndKey(t *testing.T) {
	s, _ := Open(t.TempDir(), nil)
	_ = s.Put(Scope{}, "save_cmd", "write memory", "all")
	_ = s.Put(Scope{}, "save_cmd", "copy run start", "nxos")

	if err := s.Delete(Scope{}, "save_cmd", "nxos"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Get(Scope{}, "save_cmd", "nxos"); v != "write memory" {
		t.Fatalf("after deleting nxos selector, nxos falls to all = %q", v)
	}
	if err := s.Delete(Scope{}, "save_cmd", ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get(Scope{}, "save_cmd", "all"); ok {
		t.Fatal("save_cmd should be gone after whole-key delete")
	}
}

func TestPersistenceRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir, testChain)
	_ = s.Put(Scope{}, "domain", "kages.local", "all")             // scalar form
	_ = s.Put(Scope{FolderID: "A"}, "save_cmd", "write memory", "all")
	_ = s.Put(Scope{FolderID: "A"}, "save_cmd", "copy run start", "nxos") // map form

	s2, err := Open(dir, testChain)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := s2.Get(Scope{}, "domain", "ios"); v != "kages.local" {
		t.Fatalf("persisted scalar = %q", v)
	}
	if v, _ := s2.Get(Scope{FolderID: "A"}, "save_cmd", "nxos"); v != "copy run start" {
		t.Fatalf("persisted map nxos = %q", v)
	}
	if v, _ := s2.Get(Scope{FolderID: "A"}, "save_cmd", "ios"); v != "write memory" {
		t.Fatalf("persisted map all-fallback = %q", v)
	}
	// scalar key must persist as a bare scalar, not a map
	b, _ := os.ReadFile(filepath.Join(dir, globalFile))
	if want := "domain: kages.local"; !contains(string(b), want) {
		t.Fatalf("global.yaml = %q, want scalar %q", string(b), want)
	}
}

func contains(hay, needle string) bool {
	return len(hay) >= len(needle) && (indexOf(hay, needle) >= 0)
}

func indexOf(hay, needle string) int {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
