# ADR-0004: Audit log — async pipeline, hash-chained, encrypted at rest

**Status:** accepted · 2026-07-02

## Context
Requirement: audit-proof session logging, encrypted at rest, immutable — with the
explicit constraint that logging must not slow sessions down.

## Decision
- Session goroutines never hash, encrypt, or touch disk. They emit to a bounded
  channel (structured events) and the audit writer additionally taps sealed
  scrollback chunks by reference (full-io mode, zero copy).
- One writer goroutine per log: batch -> HMAC-SHA256 hash chain
  (entryHash = HMAC(key, prevHash || entry)) -> zstd -> segment append -> fsync
  by policy (default 1s interval).
- Segments sealed at 64 MiB / 24 h: AES-256-GCM, per-segment key wrapped by a
  master key in the OS keychain (OpenBao wrapping in team phase).
- Backpressure explicit: `on_full: block|drop-and-count` (default block), queue
  sized so blocking implies real disk failure; UI banner at high-watermark first.
- `f9 audit verify` walks the chain and reports the first broken link.

## Consequences
- Tamper-evident (provable), not merely tamper-resistant — workstation-local
  "immutable" is always best-effort; hash chain is what an auditor verifies.
- Performance budget enforced by in-tree benchmark: full-io logging must cost
  < 3 % append throughput vs logging off.
