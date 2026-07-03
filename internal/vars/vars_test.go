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
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}

	must(s.Put(Scope{}, "region", "eu"))                                 // global
	must(s.Put(Scope{}, "domain", "kages.local"))                        // global
	must(s.Put(Scope{FolderID: "A"}, "region", "at"))                    // folder A overrides region
	must(s.Put(Scope{FolderID: "B"}, "vlan_id", "222"))                  // folder B
	must(s.Put(Scope{FolderID: "B", SessionID: "S1"}, "vlan_id", "999")) // session overrides

	if v, _ := s.Get(Scope{}, "region"); v != "eu" {
		t.Fatalf("global region = %q, want eu", v)
	}
	fa := s.List(Scope{FolderID: "A"})
	if fa["region"] != "at" || fa["domain"] != "kages.local" {
		t.Fatalf("folder A view = %+v", fa)
	}
	sv := s.List(Scope{FolderID: "B", SessionID: "S1"})
	if sv["region"] != "at" || sv["domain"] != "kages.local" || sv["vlan_id"] != "999" {
		t.Fatalf("session S1 view = %+v", sv)
	}
	if v, _ := s.Get(Scope{FolderID: "B"}, "vlan_id"); v != "222" {
		t.Fatalf("folder B vlan_id = %q, want 222", v)
	}
}

func TestSecretRejected(t *testing.T) {
	s, _ := Open(t.TempDir(), nil)
	for _, k := range []string{"password", "ssh_password", "API_TOKEN", "myPasswd", "vault_secret", "apikey", "private_key"} {
		if err := s.Put(Scope{}, k, "x"); err == nil {
			t.Fatalf("expected rejection for secret-like key %q", k)
		}
	}
	if !IsSecretKey("db_password") || IsSecretKey("vlan_id") {
		t.Fatal("IsSecretKey classification wrong")
	}
}

func TestKeyValidation(t *testing.T) {
	s, _ := Open(t.TempDir(), nil)
	for _, k := range []string{"", "1abc", "has space", "dash-key", "dot.key"} {
		if err := s.Put(Scope{}, k, "x"); err == nil {
			t.Fatalf("expected invalid-key error for %q", k)
		}
	}
	if err := s.Put(Scope{}, "vlan_id", "10"); err != nil {
		t.Fatalf("valid key rejected: %v", err)
	}
}

func TestPersistenceAndDelete(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir, testChain)
	_ = s.Put(Scope{}, "region", "eu")
	_ = s.Put(Scope{FolderID: "A"}, "region", "at")
	_ = s.Put(Scope{FolderID: "A", SessionID: "S1"}, "host", "sw1")

	s2, err := Open(dir, testChain) // reopen: state comes from disk
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := s2.Get(Scope{FolderID: "A", SessionID: "S1"}, "region"); v != "at" {
		t.Fatalf("persisted region = %q, want at", v)
	}
	if v, _ := s2.Get(Scope{FolderID: "A", SessionID: "S1"}, "host"); v != "sw1" {
		t.Fatalf("persisted host = %q, want sw1", v)
	}

	if err := s2.Delete(Scope{FolderID: "A", SessionID: "S1"}, "host"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s2.Get(Scope{FolderID: "A", SessionID: "S1"}, "host"); ok {
		t.Fatal("host still present after delete")
	}
	if _, err := os.Stat(filepath.Join(dir, sessionDir, "S1.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected empty session file removed; stat err = %v", err)
	}
}
