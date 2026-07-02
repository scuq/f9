// Package audit implements the tamper-evident session log: fully async
// (session goroutines only enqueue; one writer goroutine batches, hash-chains
// with HMAC-SHA256, zstd-compresses and appends to encrypted segments).
// Performance contract: full-io logging costs < 3% Append throughput
// (benchmark enforced in-tree). See ADR-0004.
package audit

import "time"

type EventType string

const (
	EvConnect    EventType = "connect"
	EvAuth       EventType = "auth"
	EvDisconnect EventType = "disconnect"
	EvSend       EventType = "send"      // user input (events+input scope)
	EvChunk      EventType = "io-chunk"  // sealed scrollback chunk by reference (full-io scope)
)

type Event struct {
	At        time.Time
	SessionID string
	Type      EventType
	Detail    string
	Chunk     []byte // only for EvChunk; referenced, not copied
}

type Config struct {
	Dir           string
	Scope         string        // off|events|events+input|full-io (inheritable via store)
	SegmentBytes  int64         // default 64 MiB
	SegmentMaxAge time.Duration // default 24h
	FsyncInterval time.Duration // default 1s
	QueueSize     int           // default 65536
	OnFull        string        // block|drop-and-count (default block)
}

type Log interface {
	Enqueue(e Event) // non-blocking up to QueueSize; policy applies beyond
	HighWatermark() (queued, capacity int)
	Close() error // flush, seal, encrypt current segment
}

func Open(cfg Config) (Log, error) { panic("phase 08: not implemented") }

// Verify walks a log dir's hash chain; returns the first broken entry, if any.
func Verify(dir string) error { panic("phase 08: not implemented") }
