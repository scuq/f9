# f9

Cross-platform SSH client — Go backend, Wails v2 + xterm.js frontend.
SecureCRT-inspired: session tree with option inheritance, million-line
searchable scrollback, virtual grep, snippets with template variables,
multi-session send with feedback, Lua-scripted session scheduler,
tamper-evident audit logging. Named after its launcher hotkey.

**Start here: [`docs/phase-plan.md`](docs/phase-plan.md)** — phases 00–09 with
implementation guides. Decisions live in [`docs/adr/`](docs/adr/).

## Status

Phase 00 (core engine) — not started. This repo is the initialized skeleton:
package contracts, configs, plan.

## Build

    make check     # build + vet + fmt + test
    make matrix    # cross-compile smoke: {linux,darwin,windows} x {amd64,arm64}

## Layout

    cmd/f9/              CLI smoke harness (phase 00e), later also headless tool
    internal/store/      session/folder store, option inheritance   (00a)
    internal/scrollback/ chunked ring buffer, grep iterator         (00b)
    internal/sshx/       transport, auth chain, jump chains         (00c)
    internal/osdetect/   passive OS fingerprinting                  (00d)
    internal/theme/      TOML schemes, iTerm2 import                (03)
    internal/grep/       virtual grep semantics                     (04)
    internal/vars/       scoped variable store                      (05)
    internal/snippets/   snippet tree + templating                  (05)
    internal/multisend/  multi-session send + feedback matrix       (06)
    internal/luaext/     gopher-lua job runtime                     (07)
    internal/audit/      async hash-chained encrypted log           (08)
    configs/             example config, os-tunings, themes
    docs/                phase plan + ADRs

Frontend (`frontend/`, Preact + xterm.js) is added in phase 01 via `wails init`.
