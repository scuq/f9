# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

f9 is a cross-platform SSH client: Go backend, Wails v2 shell, Preact + xterm.js frontend. SecureCRT-style session tree with option inheritance, jump chains, Go-side scrollback, HTTPS session import (NetBox etc.) with sandboxed Lua map scripts. Module path `github.com/scuq/f9`.

## Git

- Never run `git commit` or `git push` — the user always commits and pushes manually. Leave changes in the working tree.
- Do not add Claude as a co-author (no `Co-Authored-By` trailers) or mention Claude in commit messages, PR bodies, or anywhere else in the repo.

## Commands

```sh
make check                 # go build + vet + gofmt check + go test  (run before committing)
go test ./internal/store/  # one package
go test ./internal/sshx/ -run TestKeepalive   # one test
make matrix                # cross-compile smoke for linux/darwin/windows × amd64/arm64
make gui-dev               # Wails dev window with live reload (needs wails CLI v2.12.0, webkit2gtk-4.1 on Linux)
make gui-build             # production binary -> build/bin/f9-gui
make bump V=1.2.3          # write VERSION, commit, tag v1.2.3; `git push --follow-tags` triggers the release workflow
```

`gofmt` must be clean for `cmd` and `internal` (`make fmt` fails otherwise). Frontend: `cd frontend && npm install && npm run build` (Vite); `wails dev`/`wails build` run this for you.

## Build tags — the most important thing to know

The GUI is built only with `-tags gui,webkit2_41` (Makefile `GUI_TAGS`). A plain `go build ./...` / `go test ./...` compiles **without** the `gui` tag and must never need cgo, WebKit, or any Wails package:

- `main.go` (`//go:build gui`) is the Wails entry point; `main_stub.go` (`!gui`) is a no-op stub.
- `internal/app` imports **no** Wails packages. Anything that needs the Wails runtime is split into `*_gui.go` / `*_stub.go` pairs (`emit_gui.go`/`emit_stub.go` for `emitEvent`, `dialog_gui.go`/`dialog_stub.go` for file dialogs). Follow that pattern when adding runtime-dependent code; the stub variant also provides the `a.onEmit` test hook.
- `frontend/dist/` is **committed on purpose** — `main.go` `go:embed`s it, so a bare `go build` with the gui tag must not require npm. Rebuild and commit it when the frontend changes.
- The version string is `internal/app.Version`, stamped via `-ldflags -X` from the `VERSION` file (`"dev"` otherwise).

## Architecture

Backend packages are the engine; `internal/app` is a thin translation layer; the frontend is deliberately thin.

- **`internal/app`** — the single `App` struct bound to Wails (`Bind: []interface{}{a}`). Every exported method becomes a frontend call; it converts between engine types and JSON DTOs (`SessionNode`, `FolderNode`, `OptionField`, …) and holds no business logic. Backend → frontend push goes through `a.emitEvent("f9:...", data)`; current events: `f9:conns`, `f9:osdetected`, `f9:prompt`, `f9:termclosed`, `f9:termactivity`, `f9:themes`, `f9:multisend`, `f9:multisenddone`, `f9:xferprogress`, and per-terminal `f9:term:<termId>` data streams.
- **`internal/store`** — `Store` interface + `YAMLStore`: a directory tree under the store root where every directory is a Folder (`folder.yaml`) and every other `*.yaml` is a Session; directory structure is authoritative for parent/child. Machine-written metadata (detected OS, last connect) lives in `<root>/.meta/` so user config stays clean. Root folder options are the "Default Session". `SessionOptions` fields are pointers (nil = inherit); `resolve.go` overlays root → folder → … → session. Also: fuzzy `filter.go`, import `reconcile.go`, `source.go` (per-folder HTTPS sources), ULID ids, and every object carries `revision`/`updated_at` (ADR-0006).
- **`internal/sshx`** — transport: auth chain (agent → key files → interactive prompt; ADR-0005 — passwords are never persisted), host keys, jump chains (proxyjump vs shell-hop), keepalive/dead-link detection, SOCKS (`ssh -D`) proxy.
- **`internal/connmgr`** — lifecycle of live connections: bounded concurrent dials, per-connection state machine, state events. No terminal I/O.
- **`internal/scrollback` / `internal/grep`** — scrollback lives in Go as a chunked ring buffer (ADR-0002); xterm.js only holds a viewport window. Search is a Go-side grep iterator.
- **`internal/sessionimport` + `internal/luamap` + `internal/luaext`** — fetch/decode external sources (native / NetBox / mapped JSON), then run a sandboxed gopher-lua `map(r)` hook per record (ADR-0003: pure-Go Lua only, never CPython). User guide: `docs/import-map-scripts.md`.
- **`internal/xfer`** — SFTP engine for the upload dialog (`github.com/pkg/sftp` over the session's `*ssh.Client`): remote listing, mkdir, cancellable uploads with progress. Shell-hop targets cannot carry SFTP on the session's own client; the default route there runs `ssh -o BatchMode=yes -s user@target sftp` on the jump host (`HopClient()` + `xfer.OpenPipe`) so the hop's keys authenticate like the interactive login, with an alternative that dials a fresh SSH connection through a SOCKS-capable session's tunnel (`sshx.DialOpts.Via`, local keys). `App.Xfer*` bindings in `internal/app/xfer.go`.
- **`internal/cred`** — passphrase-locked store for *external-source* credentials only (argon2id + NaCl secretbox, single `<root>/.secrets.yaml`, opens locked; ADR-0007). SSH passwords/passphrases still never touch disk.
- **`internal/osdetect`** — passive OS fingerprinting driven by `configs/os-tunings.yaml` (a copy is embedded from `internal/app/os-tunings.yaml`).
- **`internal/vars`, `snippets`, `buttonbar`, `multisend`, `theme`, `updater`, `audit`** — variables (secret-like keys rejected), pongo2 templates + snippet library, G-Bar/C-Bar model, broadcast feedback state machine, TOML themes + iTerm2 import, GitHub release check, async hash-chained audit log (ADR-0004).
- **`cmd/f9`** — CLI harness/smoke binary (raw-mode terminal, session recording), sharing the store with the GUI.
- **`frontend/src`** — `app.tsx` is essentially the whole UI (tree, panels, modals, settings); `terminal.tsx` wraps xterm.js; `termsearch.ts`/`theme.ts` are small helpers. The frontend calls the backend via `window.go.app.App.*` and listens with `window.runtime.EventsOn`. `frontend/wailsjs/` is Wails-generated; `frontend/src/global.d.ts` is the **hand-maintained** typing of the `App` binding surface and the DTOs — update it when you add/change an exported `App` method.

Data locations: store root is `$F9_STORE` or `~/.config/f9/sessions`; `F9_RECORDINGS` (CLI), `F9_WEBVIEW_DATA` (Windows WebView2 user data). On Linux the binary forces `WEBKIT_DISABLE_DMABUF_RENDERER=1`.

## Project invariants

Recorded in `docs/adr/` and `docs/phase-plan.md`; check them before changing anything they cover:

- Backend is 100% Go; frontend stays thin (ADR-0001).
- No password/passphrase storage, ever (ADR-0005).
- Minimal dependencies — the phase plan keeps a dependency budget; don't add a Go module or npm package without a clear need.
- Style the codebase already follows: simple explicit logic, bounded loops and timeouts on every network/IO path, validate inputs at boundaries, small reviewable units.
- An import refresh that decodes 0 records must leave the tree untouched.

## Releases

Tag push `v*` runs `.github/workflows/release.yml` (5 native-runner targets, GitHub Release with generated notes); `monthly.yml` cuts an automatic patch release on the 1st. Wails CLI version is pinned there (`WAILS_VERSION`) and in the README — keep them in sync when upgrading.
