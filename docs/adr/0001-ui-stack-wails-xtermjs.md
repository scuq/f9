# ADR-0001: UI stack — Wails v2 + xterm.js

**Status:** accepted · 2026-07-02

## Context
Cross-platform (win/mac/linux, amd64/arm64) SSH client with SecureCRT-grade
terminal fidelity. Evaluated Fyne (pure Go) vs Wails + xterm.js.

## Decision
Wails v2 with a thin Preact frontend; terminal rendering by xterm.js (WebGL addon).

## Consequences
- Terminal emulation quality equals VS Code's terminal (vim/tmux/htop correct).
- Theming = CSS variables + xterm ITheme; iTerm2 scheme import trivially possible.
- Cost: native webview dependency (WebView2/WebKit) and a JS layer; mitigated by
  keeping all logic in Go — frontend renders and forwards events only.
- Fyne rejected for incomplete VT emulation in fyne-io/terminal; unacceptable for
  the core feature of a terminal product.
