package luamap

import (
	"path/filepath"
	"testing"
)

func TestLibraryRoundtrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "scripts.yaml")
	l, err := OpenLibrary(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Put("ren", `function map(r) return r end`); err != nil {
		t.Fatal(err)
	}
	if err := l.Put("", "x"); err == nil {
		t.Fatal("empty name must fail")
	}
	if err := l.Put("bad", `function map(`); err == nil {
		t.Fatal("parse error must fail")
	}
	code, ok := l.Get("ren")
	if !ok || code == "" {
		t.Fatal("Get failed")
	}
	// persistence across reopen
	l2, err := OpenLibrary(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := l2.List(); len(got) != 1 || got[0].Name != "ren" {
		t.Fatalf("List after reopen = %+v", got)
	}
	if err := l2.Delete("ren"); err != nil {
		t.Fatal(err)
	}
	if err := l2.Delete("ren"); err == nil {
		t.Fatal("double delete must fail")
	}
}
