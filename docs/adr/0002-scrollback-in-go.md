# ADR-0002: Scrollback buffer lives in Go, not xterm.js

**Status:** accepted · 2026-07-02

## Context
Requirement: millions of lines of searchable backscroll per session (full Cisco
`show run`, long-running sessions). xterm.js scrollback is DOM/JS-heap bound and
degrades far below that.

## Decision
Per-session chunked ring buffer in Go: active chunk plain bytes with line index,
sealed chunks zstd-compressed (background goroutine), optional disk spill,
caps by lines and bytes. xterm.js keeps only a viewport window (~10k lines) and
splices older lines on demand.

## Consequences
- Backscroll search, virtual grep, multi-send feedback and audit full-io capture
  all become consumers of one buffer — no duplicate storage.
- Hot path (`Append`) is memcpy-only: no locks shared with readers, no allocation,
  no crypto, no compression.
- 100 open tabs cost Go RAM (bounded, compressed), not DOM nodes.
