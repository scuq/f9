package app

import "testing"

func TestScrollbackBudget(t *testing.T) {
	if got := scrollbackBudget(0); got != 1<<30 {
		t.Fatalf("unknown RAM budget = %d", got)
	}
	if got := scrollbackBudget(8 << 30); got != 2<<30 {
		t.Fatalf("8 GiB -> %d, want 2 GiB", got)
	}
	if got := scrollbackBudget(512 << 20); got != scrollbackBudgetMin {
		t.Fatalf("512 MiB -> %d, want floor %d", got, scrollbackBudgetMin)
	}
	if got := scrollbackBudget(64 << 30); got != scrollbackBudgetMax {
		t.Fatalf("64 GiB -> %d, want ceiling %d", got, scrollbackBudgetMax)
	}
	if got := perTerminalCap(1<<30, 4); got != 256<<20 {
		t.Fatalf("1 GiB / 4 = %d", got)
	}
	if got := perTerminalCap(1<<30, 1000); got != scrollbackPerTermMin {
		t.Fatalf("tiny share must floor at %d, got %d", scrollbackPerTermMin, got)
	}
	if got := perTerminalCap(1<<30, 0); got != 1<<30 {
		t.Fatalf("zero terminals = %d", got)
	}
}
