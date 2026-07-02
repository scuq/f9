package scrollback

import (
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"testing"
)

func collect(t *testing.T, b Buffer, pattern string, opts GrepOpts) []Match {
	t.Helper()
	it, err := b.Grep(regexp.MustCompile(pattern), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()
	var out []Match
	for {
		m, ok := it.Next()
		if !ok {
			break
		}
		out = append(out, m)
	}
	if err := it.Close(); err != nil {
		t.Fatalf("iterator error: %v", err)
	}
	return out
}

func TestGrepContextAndFlags(t *testing.T) {
	b := New(Config{})
	defer b.Close()
	for _, ln := range []string{"a", "ERROR one", "b", "c", "ERROR two", "d"} {
		b.Append([]byte(ln + "\n"))
	}

	ms := collect(t, b, "ERROR", GrepOpts{Before: 1, After: 1})
	if len(ms) != 2 {
		t.Fatalf("matches = %d, want 2", len(ms))
	}
	if ms[0].LineNo != 1 || string(ms[0].Line) != "ERROR one" {
		t.Fatalf("m0 = %d %q", ms[0].LineNo, ms[0].Line)
	}
	if len(ms[0].Before) != 1 || string(ms[0].Before[0]) != "a" {
		t.Fatalf("m0.Before = %q", ms[0].Before)
	}
	if len(ms[0].After) != 1 || string(ms[0].After[0]) != "b" {
		t.Fatalf("m0.After = %q", ms[0].After)
	}
	if ms[1].LineNo != 4 || string(ms[1].Before[0]) != "c" || string(ms[1].After[0]) != "d" {
		t.Fatalf("m1 context wrong: %d %q %q", ms[1].LineNo, ms[1].Before, ms[1].After)
	}

	if got := collect(t, b, "ERROR", GrepOpts{MaxMatches: 1}); len(got) != 1 {
		t.Fatalf("MaxMatches: got %d, want 1", len(got))
	}
	if got := collect(t, b, "ERROR", GrepOpts{Invert: true}); len(got) != 4 {
		t.Fatalf("Invert: got %d, want 4", len(got))
	}
	if got := collect(t, b, "error", GrepOpts{IgnoreCase: true}); len(got) != 2 {
		t.Fatalf("IgnoreCase: got %d, want 2", len(got))
	}
}

// TestGrepMatchesNaive is the property test: streaming grep over sealed +
// active chunks must equal a naive grep over an uncompressed mirror.
func TestGrepMatchesNaive(t *testing.T) {
	rng := rand.New(rand.NewSource(9933))
	words := []string{"interface", "shutdown", "vlan", "ERROR", "up", "down", "GigabitEthernet", "noise"}

	var corpus []string
	for i := 0; i < 5000; i++ {
		n := 1 + rng.Intn(5)
		parts := make([]string, n)
		for j := range parts {
			parts[j] = words[rng.Intn(len(words))]
		}
		corpus = append(corpus, fmt.Sprintf("%04d %s", i, strings.Join(parts, " ")))
	}

	b := New(Config{ChunkSize: 4096}) // small chunks: force many seals
	defer b.Close()
	blob := []byte(strings.Join(corpus, "\n") + "\n")
	for len(blob) > 0 { // append in random-sized writes
		n := 1 + rng.Intn(700)
		if n > len(blob) {
			n = len(blob)
		}
		b.Append(blob[:n])
		blob = blob[n:]
	}

	for _, tc := range []struct {
		pattern string
		invert  bool
	}{
		{"ERROR", false},
		{"ERROR", true},
		{"^0.* vlan", false},
		{"GigabitEthernet.*down", false},
	} {
		re := regexp.MustCompile(tc.pattern)
		var want []int
		for i, ln := range corpus {
			m := re.MatchString(ln)
			if tc.invert {
				m = !m
			}
			if m {
				want = append(want, i)
			}
		}
		got := collect(t, b, tc.pattern, GrepOpts{Invert: tc.invert})
		if len(got) != len(want) {
			t.Fatalf("%q invert=%v: %d matches, want %d", tc.pattern, tc.invert, len(got), len(want))
		}
		for i := range want {
			if got[i].LineNo != want[i] {
				t.Fatalf("%q match %d at line %d, want %d", tc.pattern, i, got[i].LineNo, want[i])
			}
			if string(got[i].Line) != corpus[want[i]] {
				t.Fatalf("%q line content mismatch at %d", tc.pattern, want[i])
			}
		}
	}
}
