package filter

import (
	"fmt"
	"testing"
	"time"
)

func items10k() []Item {
	out := make([]Item, 0, 10000)
	for i := 0; i < 10000; i++ {
		out = append(out, Item{
			ID:   fmt.Sprintf("id%05d", i),
			Name: fmt.Sprintf("sw%04d-core", i),
			Path: fmt.Sprintf("Sessions/dc%d/rack%02d", i%4, i%40),
			Host: fmt.Sprintf("10.21.%d.%d", i/250, i%250),
			Tags: []string{"cisco", "access"},
		})
	}
	return out
}

func TestRankBasics(t *testing.T) {
	items := []Item{
		{ID: "1", Name: "sw-lab-01", Path: "Sessions/lab", Host: "127.0.0.1"},
		{ID: "2", Name: "nx-behind-jumps", Path: "Sessions/lab", Host: "10.21.200.10"},
		{ID: "3", Name: "dev-behind-bastion", Path: "Sessions/lab", Host: "10.21.201.5"},
	}
	hits := Rank("swlab", items)
	if len(hits) != 1 || hits[0].ID != "1" {
		t.Fatalf("subsequence swlab: %+v", hits)
	}
	hits = Rank("behind", items)
	if len(hits) != 2 {
		t.Fatalf("substring behind: want 2 hits, got %+v", hits)
	}
	hits = Rank("10.21.20", items)
	if len(hits) != 2 || hits[0].Score <= 0 {
		t.Fatalf("host match: %+v", hits)
	}
	hits = Rank("lab", items)
	if len(hits) != 3 {
		t.Fatalf("path match should hit all lab sessions: %+v", hits)
	}
	// name substring must outrank path substring
	if hits[0].ID != "1" {
		t.Fatalf("name hit should rank first for 'lab': %+v", hits)
	}
	if got := Rank("", items); len(got) != 3 {
		t.Fatalf("empty query: want all, got %d", len(got))
	}
	if got := Rank("zzz", items); len(got) != 0 {
		t.Fatalf("no-match query: got %+v", got)
	}
}

func TestPrefixBeatsMidstring(t *testing.T) {
	items := []Item{
		{ID: "mid", Name: "core-sw01"},
		{ID: "pre", Name: "sw01-core"},
	}
	hits := Rank("sw01", items)
	if len(hits) != 2 || hits[0].ID != "pre" {
		t.Fatalf("prefix should rank first: %+v", hits)
	}
}

// TestFilterBudget enforces the plan's <5ms/10k contract.
func TestFilterBudget(t *testing.T) {
	items := items10k()
	queries := []string{"sw12", "rack03", "10.21.7", "core", "swcore"}
	const runs = 50
	start := time.Now()
	for i := 0; i < runs; i++ {
		Rank(queries[i%len(queries)], items)
	}
	avg := time.Since(start) / runs
	t.Logf("avg Rank over 10k items: %v", avg)
	if avg > 5*time.Millisecond {
		t.Fatalf("filter budget blown: %v per query (budget 5ms)", avg)
	}
}

func BenchmarkRank10k(b *testing.B) {
	items := items10k()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Rank("sw12", items)
	}
}
