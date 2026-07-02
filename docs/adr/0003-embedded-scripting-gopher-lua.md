# ADR-0003: Embedded scripting — gopher-lua

**Status:** accepted · 2026-07-02

## Context
Scheduler needs an embedded language with HTTP for session import/update/delete
jobs (CMDB/NetBox reconciliation). Candidates: Lua (gopher-lua) vs embedded Python.

## Decision
gopher-lua (+ gluahttp). Pure Go: keeps CGO-free cross-compilation for all six
GOOS/GOARCH targets. One VM per job run, instruction budget + wallclock timeout,
filesystem access only via explicit sandbox dir.

## Consequences
- No CPython embedding pain on 6 targets.
- Users needing Python later: external script hook (exec + JSON over stdio),
  not embedding.
