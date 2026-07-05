# f9

A cross-platform SSH client with a Go backend and a Wails v2 + xterm.js (Preact)
frontend. SecureCRT-inspired: a session tree with option inheritance, searchable
scrollback, template-driven sending, multi-session broadcast with a feedback
matrix, and folder-level session import from external HTTPS sources. Named after
its launcher hotkey.

![f9](docs/screenshot.png)

## Features

- **Session tree** — folders with inherited connection options; jump-host chains.
- **Terminals** — xterm.js with chunked scrollback and a virtual grep panel (Ctrl/Cmd+F).
- **Passive OS detection** — per-session prompt/error tuning from `configs/os-tunings.yaml` (embedded in the binary).
- **Themes** — TOML colour schemes, iTerm2 import.
- **Variables & templates** — scoped, OS-tagged variables (secret keys rejected); pongo2 templates; prompt-for-unresolved on send.
- **Button bars** — a global **G-Bar** and a contextual **C-Bar** (folder / detected-OS); send / snippet / launch / URL actions; horizontal or vertical (pin & collapse) layout.
- **Snippet library** — standalone tree, editor, and a Ctrl/Cmd+P fuzzy picker.
- **Multi-send** — broadcast a line or template to marked tabs with a live per-target feedback matrix (sent → echoed → ok / error / timeout), dry-run, and guard rails.
- **External session import** — per-folder HTTPS source (f9-native / NetBox / mapped JSON) with bearer / HTTP-basic / mTLS auth; credentials encrypted at rest (argon2id + NaCl secretbox); reconcile by hostname or external id. The root stays local; one source per subtree.
- **Update check** — polls GitHub Releases and offers a download when a newer build is available.

Feature bars (button bars, templates, snippets, multi-send, vertical layout) are
off by default and enabled in Settings.

## Build & run

Requires Go 1.25+, Node 20+, and the Wails v2 CLI. On Debian/Ubuntu the GUI needs
WebKitGTK 4.1:

    sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.1-dev
    go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0

Then:

    make check        # go build + vet + gofmt + test
    make gui-dev      # run the GUI in dev mode
    make gui-build    # build the GUI binary -> build/bin/f9-gui

The in-app version comes from the `VERSION` file, injected at build time via
`-ldflags`.

## Versioning & releases

- `make bump V=1.2.3` — writes `VERSION`, commits, and tags `v1.2.3`.
- `git push --follow-tags` triggers the release workflow: it builds five targets
  (linux amd64/arm64, windows amd64/arm64, macOS arm64) on native GitHub runners
  and publishes a GitHub Release with the assets.
- A weekly workflow cuts an automatic patch release each Sunday.

### Runtime dependencies

Wails uses the platform webview, so the binaries are not fully static:

- **Linux** — needs `libwebkit2gtk-4.1-0` and `libgtk-3-0` installed.
- **Windows** — needs the WebView2 runtime (present on Windows 11 and most Windows 10).
- **macOS** — self-contained `.app` (system WKWebView).

## Layout

    cmd/f9/                  CLI harness
    internal/store/          session/folder store, option inheritance, import source
    internal/scrollback/     chunked ring buffer, grep iterator
    internal/sshx/           transport, auth chain, jump chains
    internal/osdetect/       passive OS fingerprinting + tunings
    internal/vars/           scoped, OS-tagged variables
    internal/snippets/       template rendering + snippet library
    internal/buttonbar/      G-Bar / C-Bar model
    internal/multisend/      broadcast feedback state machine
    internal/sessionimport/  HTTPS fetch + decode (native / netbox / mapped)
    internal/cred/           passphrase-locked credential store (argon2id + secretbox)
    internal/updater/        GitHub release update check
    internal/theme/          TOML schemes, iTerm2 import
    internal/app/            Wails bindings
    frontend/                Preact + xterm.js UI

Architecture decisions live in [`docs/adr/`](docs/adr/).

## License

f9 is licensed under the **GNU General Public License v3.0** — see [`LICENSE`](LICENSE).

All third-party dependencies are under GPLv3-compatible licenses (MIT,
BSD-2/3-Clause, Apache-2.0), and the bundled fonts (Inter, JetBrains Mono) under
the SIL Open Font License 1.1. Generate a full dependency license manifest with:

    go install github.com/google/go-licenses@latest
    go-licenses report ./... > THIRD_PARTY_LICENSES.txt
