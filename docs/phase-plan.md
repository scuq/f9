# f9 — Phase Plan

Cross-platform SSH client (Windows / macOS / Linux, amd64 + arm64).
Go backend, Wails v2 shell, xterm.js terminal. SecureCRT-inspired UX.

Repo: `github.com/scuq/f9`

Invariants (see ADRs, `docs/adr/`):

- **ADR-0001** UI stack: Wails v2 + xterm.js. Backend 100% Go, frontend thin (Preact + xterm.js).
- **ADR-0002** Scrollback lives in Go (chunked ring buffer, millions of lines). xterm.js holds only a viewport window.
- **ADR-0003** Embedded scripting: gopher-lua (pure Go). No CPython embedding, ever.
- **ADR-0004** Audit log: fully async off the hot path, hash-chained, encrypted at rest.
- **ADR-0005** No password storage. Agent, key files, or interactive prompt only.
- **ADR-0006** Session store carries `revision`/`updated_at` from day one (team sync is a later phase, not a retrofit).

Coding guidelines apply throughout: simple explicit logic, bounded loops/timeouts,
small reviewable units, validate all boundaries, minimal dependencies, `go vet` +
`gofmt` clean, warnings are errors.

Dependency budget (add only when the phase needs it):

| Dep | Phase | Purpose |
|---|---|---|
| `golang.org/x/crypto/ssh` (+ `agent`, `knownhosts`) | 00 | transport, auth, host keys |
| `golang.org/x/term` | 00 | CLI raw mode + hidden prompts |
| `gopkg.in/yaml.v3` | 00 | YAML persistence for the session store |
| `github.com/klauspost/compress/zstd` | 00 | scrollback + audit segment compression |
| `github.com/wailsapp/wails/v2` | 01 | app shell |
| `github.com/sahilm/fuzzy` | 01 | session filter scoring (or hand-rolled subsequence scorer) |
| `github.com/flosch/pongo2/v6` | 05 | jinja2-style snippet templating |
| `github.com/yuin/gopher-lua` + `gluahttp` | 07 | scheduler scripting |
| `github.com/robfig/cron/v3` | 07 | schedules |
| `github.com/zalando/go-keyring` | 08 | audit master key in OS keychain |

Frontend deps (phase 01+): preact, xterm.js (`@xterm/xterm`), `@xterm/addon-fit`,
`@xterm/addon-webgl`, `@xterm/addon-search` (viewport-local search only).

---

## Phase 00 — Core engine (no GUI)

**Goal:** everything that must be correct before pixels exist. Exit with a CLI
smoke binary (`cmd/f9 --connect <session>`) proving transport, store, scrollback,
OS detection and jumphosts.

### 00a — Session store (`internal/store`)

- On-disk layout: one YAML file per folder OR single SQLite db. **Decision: start
  with a directory tree of YAML** (git-diffable, matches CMDB-export workflows);
  the store is behind an interface so SQLite can replace it if 10k-file I/O ever hurts.
- Model:
  - `Folder{ID, Name, ParentID, Options SessionOptions, Revision, UpdatedAt}`
  - `Session{ID, Name, FolderID, Host, Port, User, Proto, Options SessionOptions, Meta SessionMeta, Revision, UpdatedAt}`
  - `SessionOptions` — every field a *pointer* (nil = inherit). Effective options =
    walk root→folder→session, overlay non-nil fields. One function:
    `Resolve(defaults Options, chain ...Options) Options`. Test this exhaustively;
    it is the SecureCRT "Default Session" semantics.
  - `SessionMeta` — mutable, machine-written (detected OS, last connect, tunings).
    Stored separately (`.meta/` sidecar files) so user config stays clean and
    git-friendly.
- In-memory index: load all sessions at startup into a flat slice + folder tree.
  10k sessions ≈ a few MB. All filtering happens here, never on disk.
- IDs: ULIDs (sortable, no coordination needed for later sync).

### 00b — Scrollback ring buffer (`internal/scrollback`)

The single most load-bearing component. Consumers: terminal viewport, search,
virtual grep, multi-send feedback, audit I/O capture — all read the same buffer.

- Structure: `Buffer` = ordered list of `Chunk`s. Active chunk = plain `[]byte`
  (line-indexed as it fills). Sealed chunks (default 1 MiB) are zstd-compressed
  in a background goroutine; buffer keeps `[]sealedChunk{compressed, lineCount, byteRange}`.
- Line index: per chunk, offsets of `\n` positions (`[]uint32`) so "line N" and
  "last M lines" are O(log n) lookups without decompression bookkeeping.
- Caps: configurable by lines AND bytes (default: 5M lines / 512 MiB compressed);
  overflow policy = drop oldest chunk, optionally spill to disk (temp dir,
  session-scoped, wiped on close unless logging is on).
- API sketch:
  ```go
  Append(p []byte)                          // hot path: memcpy into active chunk, nothing else
  Lines(from, to int) ([][]byte, error)     // decompress-on-read, LRU of 4 hot chunks
  Grep(re *regexp.Regexp, opts GrepOpts) (Iterator, error)  // streaming, context lines A/B, invert
  Len() (lines int, bytes int64)
  ```
- Hot-path rule: `Append` does no allocation beyond chunk growth, no locks shared
  with readers (chunk seal = atomic swap). Benchmark target: ≥ 500 MB/s append
  single-core; a `show run` flood must never backpressure the SSH channel.
- Tests: fuzz line indexing; property test `Lines(Grep(...))` equals naive
  grep over an uncompressed mirror.

### 00c — SSH transport (`internal/sshx`)

- Wrap `x/crypto/ssh`: `Dialer` (timeouts, keepalives `ServerAliveInterval`-style),
  `Client`, `Session{Stdin io.Writer, attach(onData func([]byte))}` — `onData`
  fans out to scrollback + (later) xterm stream + multi-send matcher.
- Auth chain, in order: ssh-agent (`SSH_AUTH_SOCK` / Windows named pipe
  `\\.\pipe\openssh-ssh-agent`) → key files (encrypted keys: passphrase prompt
  callback) → keyboard-interactive/password prompt callback. **Nothing persisted**
  (ADR-0005). Prompt callbacks are interfaces so CLI and Wails provide their own.
- Host keys: `knownhosts` with TOFU flow — unknown key returns a typed error the
  UI turns into an accept/reject dialog; accepted keys appended to f9's own
  `known_hosts`.
- Jumphosts, both modes (per session or inherited from folder):
  1. **proxyjump**: dial hop, `hopClient.Dial("tcp", target)`, wrap conn in new
     handshake. Chain of N hops = fold.
  2. **shell-hop**: open shell on hop, expect prompt (reuse osdetect matchers),
     send `ssh -o ... user@target`, hand the shell's stdin/stdout to the session
     as its transport. Username substitution per hop (`user_override`).
     Needed for bastions that forbid TCP forwarding.
- Keepalive + reconnect policy in options (`keepalive_interval`, `reconnect: off|prompt|auto`).

### 00d — Passive OS detection (`internal/osdetect`)

- **Never inject bytes.** Fingerprint from what flows anyway:
  server version string (`SSH-2.0-Cisco-1.25`, `OpenSSH_9.x`), login banner,
  prompt shape (`>` / `#` / `$`, `hostname(config)#`), pager markers
  (`--More--`, `lines 1-45`), error idioms (`% Invalid input`).
- Scoring table → `OSGuess{Family: ios|nxos|panos|junos|linux|openbsd|windows, Confidence}`.
  Written to `SessionMeta` once confidence ≥ threshold; user can pin/override.
- Consumers: tuning profiles keyed by family (term type, send-newline style CR/LF,
  prompt regex for multi-send feedback, default highlight set). Profiles are data
  (`configs/os-tunings.yaml`), not code.

### 00e — CLI smoke binary (`cmd/f9`)

`f9 list`, `f9 connect <name>` (raw stdio attach), `f9 grep <name> <re>` against
a recorded buffer. Proves 00a–00d end to end. Keep it; it becomes the headless/CI
test harness forever.

**Exit criteria:** connect through 2-hop proxyjump and shell-hop to lab devices;
1M-line synthetic `show run` appended with zero drops; osdetect correct on
IOS/NX-OS/Linux/OpenBSD lab set; `go test ./...` green on all 6 GOOS/GOARCH targets
(cross-compile check in CI).

---

## Phase 01 — Wails shell + session manager

**Goal:** the left half of the SecureCRT screenshot.

- `wails init` (Preact + TS template) layered onto this repo; backend bindings live
  in `internal/app` (thin — translate UI calls to store/sshx, no logic).
- Session tree: virtualized list (render visible rows only; the tree is flattened
  to visible-row array on expand/collapse — 10k rows stay trivial).
- Filter: score over `folderPath + name + host + tags`, subsequence match with
  word-boundary bonus (or `sahilm/fuzzy`), debounce 30 ms, run in Go, return
  IDs + match ranges for highlight. Budget: < 5 ms for 10k @ p99.
- Session CRUD dialogs incl. the inheritance view ("effective value — inherited
  from folder X" like SecureCRT's greyed options).
- Multisession connect: select folder / mark N sessions → worker pool
  (default 8 parallel dials, configurable), progress toast, per-session result list.
- Active-sessions panel with its own filter (same scorer over live sessions).
- **Favorite tabs groundwork:** tab model has `pinned bool` now, UI in phase 02.

**Exit:** create/edit/filter/connect all work; 100-session folder connect completes
with sane throttling; window state persists.

---

## Phase 02 — Terminal

- xterm.js + WebGL addon; one `Terminal` per tab, but **only attached tabs render**;
  background tabs just accumulate in the Go buffer (that's ADR-0002 paying rent —
  100 open tabs cost RAM in Go, not DOM).
- Data path: Wails events carry base64 chunks (batch ≤ 16 ms) Go→JS; JS→Go stdin
  via bound method. Resize propagates to PTY req.
- Viewport protocol: xterm.js keeps last N (default 10k) lines; scrolling past top
  requests older lines from Go buffer (infinite-scroll splice). Search-in-viewport
  via addon; full-history search is phase 04.
- Tab strip: favorites row always visible (pinned tabs never scroll out; overflow
  scroller only for unpinned), status dot per tab (connected / reconnecting / dead),
  middle-click close, Ctrl+Shift+F filter-jump across open tabs.
- Bell, title escape (OSC 0/2) → tab title, configurable.

**Exit:** vim/tmux/htop flawless; 100 tabs open with 10 active floods → UI stays
responsive; pinned tabs behave.

---

## Phase 03 — Theming

- One TOML scheme file = GUI palette + tree colors + terminal 16/256 palette +
  fonts + background (color/image/opacity) + cursor + selection:
  `configs/themes/*.toml` (ship `default-dark`, `cyberpunk-neon`, `oled-black`).
- GUI side: scheme → CSS custom properties injected at root; tree/tab/status colors
  all var-driven (no hardcoded colors in components — lint for `#` literals in CSS).
- Terminal side: scheme → xterm.js `ITheme` + font settings.
- iTerm2 `.itermcolors` importer (plist → palette section) — instant theme library.
- Per-session override: `Options.ThemeRef` (inheritable like everything else) —
  prod sessions red-tinted, lab green, the classic trick.
- Live reload: file watcher on themes dir; edits apply without restart.

**Exit:** switch theme at runtime affecting GUI+tree+terminals; import an
.itermcolors; per-folder theme override works.

---

## Phase 04 — Output tooling (highlight / search / virtual grep)

- **Highlight rules:** list of `{regex, fg, bg, bold, scope: global|folder|session|osfamily}`.
  Compiled once, applied in JS on the render path via xterm.js decorations for the
  viewport only (Go is not in the render loop). Rule editor with live preview +
  test string. Ship a starter set (`error|fail|down` red, `up|success` green,
  IP/MAC/interface patterns).
- **Backscroll search:** modal over full Go buffer — `scrollback.Grep` streaming
  results (line no + context), click result → viewport jumps (splice protocol from
  phase 02). Regex or literal, case toggle.
- **Virtual grep:** the killer feature. Overlay/modal per tab:
  input mimics real grep: pattern + `-v -i -A n -B n -E`, applied to *entire*
  buffer or selection; results in a scrollable pane that can itself be re-grepped
  (pipe chaining: breadcrumb `show run | grep interface | grep -v shut`);
  export result to file/clipboard/new-scratch-tab. All server-side (Go), streamed,
  cancelable (context), bounded memory (iterator, not slice).

**Exit:** grep a 1M-line buffer < 1 s with streaming first-results < 50 ms;
chained grep; highlight rules live on scroll with no fps drop.

---

## Phase 05 — Button bar + snippets + vars store

- **Vars store (`internal/vars`):** scoped KV: global → folder → session (same
  resolve semantics as options). Values string or list; secrets NOT allowed here
  (enforced: no `password`-named keys, docs point at agent/prompt).
  Lua (phase 07) gets read access: `vars.get("scope", "key")`.
- **Snippets (`internal/snippets`):** tree of snippets (folders, like sessions).
  Multiline body, rendered with pongo2 (`{{ var }}`, `{% for %}`, filters).
  Unresolved vars → prompt dialog at paste time (with per-var remember-for-session).
  Paste modes: type (with configurable inter-line delay — Cisco config paste!),
  bracketed-paste, send-on-Ctrl+Enter. Snippet picker: same fuzzy scorer, Ctrl+P style.
- **Button bar:** rows of user buttons `{icon, label, color, action}`;
  actions: send-string (pongo2-rendered, so vars work), run-snippet, launch-app
  (exec, no shell interp), open-url, internal-command (e.g. `grep-overlay`).
  Per-folder button-bar override (inheritable). Import/export as YAML.

**Exit:** snippet with `{{ vlan_id }}` prompts and pastes multiline with delay;
button launches app and sends templated string; bars switch with session folder.

---

## Phase 06 — Multi-send + feedback

- Mark tabs (checkbox in tab strip / "all in folder" / all): send one line or a
  snippet to all marked.
- **Feedback matrix** (the hard part): per tab a state machine
  `sent → echoed (input mirrored in output) → prompt-returned (osdetect prompt
  regex matched) → ok | error-pattern | timeout`. Results grid: session × state ×
  captured output tail; click cell → jump to that tab at that buffer position.
  Uses only passive matching (prompt regexes from os-tunings) — consistent with 00d.
- Guard rails: confirm dialog when > N targets or when any target's osfamily
  mismatches the majority; dry-run mode (render template per session, show, don't send).
- Sequential mode option (send to next only after previous prompt-returned) for
  fragile targets.

**Exit:** send `show clock` to 20 lab sessions, matrix goes green with per-session
timing; one dead session times out without stalling others.

---

## Phase 07 — Scheduler + Lua

- `internal/luaext`: gopher-lua VM per job run (cheap, isolated), stdlib whitelist,
  `http` module (gluahttp, timeouts mandatory), and f9 API:
  ```lua
  sessions.list(filter)  sessions.create(tbl)  sessions.update(id, tbl)
  sessions.delete(id)    folders.ensure(path)  vars.get(scope, key)
  log.info(msg)          f9.version()
  ```
  All writes go through the store API → revisions bump → UI refreshes via event.
- Scheduler: `robfig/cron` specs + on-start + manual run; jobs are Lua files in
  `~/.config/f9/jobs/` with a YAML header (schedule, enabled, timeout).
  Every run: captured stdout/stderr, duration, result → job history view.
- Canonical use case (ship as example): pull CMDB/NetBox export over HTTP,
  reconcile folder `cmdb/` (create/update/delete sessions) — the screenshot's
  `cmdb/00-JUMPHOSTS...` tree, automated.
- Hard limits: VM instruction budget + wallclock timeout + no filesystem access
  except an explicit `data/` sandbox.

**Exit:** example NetBox-style JSON → session tree sync job runs on schedule,
idempotent, history visible.

---

## Phase 08 — Audit logging (encrypted, immutable, **zero hot-path cost**)

Design directly answers "nothing slows down because of logging":

- **Hot path does one thing:** the `onData` fan-out already hands chunks to the
  scrollback buffer. Audit taps the *sealed-chunk* event (already off hot path)
  plus a bounded channel of structured events (connect/auth/disconnect/send).
  No hashing, no crypto, no disk I/O on the session goroutine. Ever.
- Writer pipeline (one goroutine per log): batch (100 ms / 64 KiB) → hash-chain
  `entryHash = HMAC-SHA256(key, prevHash || canonicalEntry)` → zstd → append to
  segment file → fsync policy (`interval|every-batch`, default interval 1 s).
- Segments (default 64 MiB or 24 h): sealed = header{firstHash,lastHash,count} +
  AES-256-GCM encrypt (per-segment key wrapped by master key from OS keychain;
  OpenBao wrap in team phase). Verify tool: `f9 audit verify <dir>` walks the chain,
  reports first broken link.
- Backpressure policy explicit in config: `audit.on_full: block|drop-and-count`.
  Default `block` with a channel sized (64k events) such that blocking indicates a
  real disk problem — and a UI banner fires *before* that (queue high-watermark).
- Scope config: events-only | events+input | full-io, per folder/session
  (inheritable). Full-io reuses scrollback chunks by reference — no double copy.
- Benchmarks in-tree: `Append` throughput with audit off vs full-io must differ
  < 3 %.

**Exit:** flood test shows < 3 % delta; `verify` detects a flipped bit; keys never
on disk unwrapped.

---

## Phase 09 — Team sync (deferred, design-only for now)

- Server: Go, Postgres, session/folder/vars replication with `revision` +
  vector-clock-lite (last-writer-wins per field, conflicts surfaced in UI).
- Sharing: folders as shared roots with ACLs; no credentials sync ever (ADR-0005
  makes this easy — nothing secret in the store).
- Transport: HTTPS + device keys; offline-first, sync is a background job.
- Not started until 00–08 are stable. Store schema already carries what it needs.

---

## Cross-cutting

- **CI (GitHub Actions):** matrix `{windows,darwin,linux} × {amd64,arm64}` build +
  `go vet` + tests; frontend lint/build; wails package on tags.
- **Config root:** `~/.config/f9/` (XDG), `%APPDATA%\f9\` on Windows;
  everything YAML/TOML, everything git-friendly.
- **Shortcut, for the record:** global launcher hotkey default = **F9**. Obviously.
