// Package scrollback implements the per-session chunked ring buffer that backs
// terminal history, search, virtual grep, multi-send feedback and audit full-io
// capture (ADR-0002).
//
// Hot-path contract: Append performs a memcpy plus newline indexing under a
// briefly-held mutex — no compression, no crypto, no disk I/O, no allocation
// beyond chunk growth. Compression of sealed chunks happens on a background
// goroutine; readers snapshot immutable chunk views and decompress outside the
// append path. The contract is enforced by BenchmarkAppend (target: >= 500 MB/s
// single-core).
package scrollback

import "regexp"

type Config struct {
	ChunkSize int   // sealed-chunk target size; default 1 MiB
	MaxLines  int   // retained-line cap; default 5_000_000
	MaxBytes  int64 // retained-byte cap (compressed + active); default 512 MiB

	// SpillToDisk/SpillDir: overflow spills to disk instead of dropping the
	// oldest chunk. TODO(phase 00b follow-up): not implemented yet; fields
	// reserved so configs stay stable.
	SpillToDisk bool
	SpillDir    string
}

type GrepOpts struct {
	Invert     bool // -v
	IgnoreCase bool // -i: best-effort (?i) recompile; prefer compiling the pattern case-insensitively
	After      int  // -A n
	Before     int  // -B n
	MaxMatches int  // 0 = unlimited
}

// Match is one grep hit with its context window. All byte slices are copies
// owned by the caller.
type Match struct {
	LineNo int // absolute line number since buffer creation
	Line   []byte
	Before [][]byte
	After  [][]byte
}

// Iterator streams matches without materializing the result set.
type Iterator interface {
	Next() (Match, bool)
	Close() error
}

// Buffer line numbers are absolute since creation: the retained window is
// [FirstLine(), FirstLine()+lines) with lines from Len(). A trailing partial
// line (no newline yet — e.g. a prompt) counts as a line; returned lines never
// include their trailing newline.
type Buffer interface {
	Append(p []byte)                      // HOT PATH — see package doc
	Lines(from, to int) ([][]byte, error) // [from, to)
	Grep(re *regexp.Regexp, opts GrepOpts) (Iterator, error)
	Len() (lines int, bytes int64)
	FirstLine() int
	// OnSeal registers a callback fired (off the hot path) whenever a chunk is
	// compressed: the audit writer consumes compressed chunks by reference
	// (zero copy). chunk must be treated as immutable.
	OnSeal(func(chunk []byte, firstLine, lastLine int))
	Close() error
}

// New returns the chunked ring buffer implementation.
func New(cfg Config) Buffer { return newRing(cfg) }
