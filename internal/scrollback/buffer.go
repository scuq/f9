// Package scrollback implements the per-session chunked ring buffer that backs
// terminal history, search, virtual grep, multi-send feedback and audit full-io
// capture. Hot-path contract: Append is memcpy-only — no locks shared with
// readers, no allocation beyond chunk growth, no compression, no crypto.
// See docs/phase-plan.md 00b and ADR-0002.
package scrollback

import "regexp"

type Config struct {
	ChunkSize   int   // sealed-chunk target size, default 1 MiB
	MaxLines    int   // default 5_000_000
	MaxBytes    int64 // compressed cap, default 512 MiB
	SpillToDisk bool  // overflow spills instead of dropping oldest
	SpillDir    string
}

type GrepOpts struct {
	Invert     bool // -v
	IgnoreCase bool // -i (compile pattern accordingly)
	After      int  // -A
	Before     int  // -B
	MaxMatches int  // 0 = unlimited
}

// Match is one grep hit with its context window.
type Match struct {
	LineNo  int
	Line    []byte
	Before  [][]byte
	After   [][]byte
}

// Iterator streams matches without materializing the result set.
type Iterator interface {
	Next() (Match, bool)
	Close() error
}

type Buffer interface {
	Append(p []byte)                    // HOT PATH — see package doc
	Lines(from, to int) ([][]byte, error)
	Grep(re *regexp.Regexp, opts GrepOpts) (Iterator, error)
	Len() (lines int, bytes int64)
	// SealedChunks lets the audit writer consume compressed chunks by
	// reference (zero copy). Registered callback runs off the hot path.
	OnSeal(func(chunk []byte, firstLine, lastLine int))
	Close() error
}

// New returns the chunked ring buffer implementation (phase 00b).
func New(cfg Config) Buffer { panic("phase 00b: not implemented") }
