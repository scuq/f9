package store

import (
	"strings"
	"testing"
	"time"
)

func TestULIDShape(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := NewULID()
		if len(id) != 26 {
			t.Fatalf("len = %d, want 26 (%s)", len(id), id)
		}
		for _, c := range id {
			if !strings.ContainsRune(ulidAlphabet, c) {
				t.Fatalf("invalid char %q in %s", c, id)
			}
		}
		if seen[id] {
			t.Fatalf("duplicate ulid %s", id)
		}
		seen[id] = true
	}
}

func TestULIDSortableByTime(t *testing.T) {
	a := NewULID()
	time.Sleep(3 * time.Millisecond)
	b := NewULID()
	if a >= b {
		t.Fatalf("expected %s < %s", a, b)
	}
}
