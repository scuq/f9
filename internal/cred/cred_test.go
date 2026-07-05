package cred

import (
	"path/filepath"
	"testing"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), ".secrets.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSetUnlock(t *testing.T) {
	s := newStore(t)
	if s.Initialized() {
		t.Fatal("should be uninitialized")
	}
	if err := s.SetPassphrase("correct horse"); err != nil {
		t.Fatal(err)
	}
	if !s.Initialized() || s.Locked() {
		t.Fatal("should be initialized + unlocked after set")
	}
	if err := s.SetPassphrase("again"); err != ErrInit {
		t.Fatalf("re-set should fail ErrInit, got %v", err)
	}
	s.Lock()
	if !s.Locked() {
		t.Fatal("should be locked")
	}
	if err := s.Unlock("wrong"); err != ErrPassphrase {
		t.Fatalf("wrong pass -> ErrPassphrase, got %v", err)
	}
	if err := s.Unlock("correct horse"); err != nil {
		t.Fatal(err)
	}
	if s.Locked() {
		t.Fatal("should be unlocked")
	}
}

func TestPutGetLocked(t *testing.T) {
	s := newStore(t)
	if err := s.Put("f1", "tok"); err != ErrLocked {
		t.Fatalf("put while locked -> ErrLocked, got %v", err)
	}
	if err := s.SetPassphrase("pw"); err != nil {
		t.Fatal(err)
	}
	if err := s.Put("f1", "token-123"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("f1")
	if err != nil || got != "token-123" {
		t.Fatalf("get = %q, %v", got, err)
	}
	if _, err := s.Get("nope"); err != ErrNotFound {
		t.Fatalf("missing -> ErrNotFound, got %v", err)
	}
	s.Lock()
	if _, err := s.Get("f1"); err != ErrLocked {
		t.Fatalf("get while locked -> ErrLocked, got %v", err)
	}
}

func TestPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".secrets.yaml")
	s1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s1.SetPassphrase("pw"); err != nil {
		t.Fatal(err)
	}
	if err := s1.Put("f1", "secret-value"); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(path) // fresh from disk
	if err != nil {
		t.Fatal(err)
	}
	if !s2.Initialized() {
		t.Fatal("should load initialized")
	}
	if !s2.Locked() {
		t.Fatal("fresh open should be locked")
	}
	if err := s2.Unlock("pw"); err != nil {
		t.Fatal(err)
	}
	got, err := s2.Get("f1")
	if err != nil || got != "secret-value" {
		t.Fatalf("persisted get = %q, %v", got, err)
	}
}

func TestDelete(t *testing.T) {
	s := newStore(t)
	if err := s.SetPassphrase("pw"); err != nil {
		t.Fatal(err)
	}
	if err := s.Put("f1", "x"); err != nil {
		t.Fatal(err)
	}
	if !s.Has("f1") {
		t.Fatal("should have f1")
	}
	if err := s.Delete("f1"); err != nil {
		t.Fatal(err)
	}
	if s.Has("f1") {
		t.Fatal("f1 should be gone")
	}
}
