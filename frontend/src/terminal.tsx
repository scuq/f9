import { useEffect, useRef } from "preact/hooks";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";

// Phase 02a: xterm.js DOM renderer (sharpest on the WebKitGTK SHM path; the
// million-line history lives in Go, so xterm only ever renders the viewport).
// Terminals persist while their connection is alive — hidden, not unmounted —
// so switching sessions never kills a shell.
//
// Resize is debounced and deduplicated: a TermResize (-> remote SIGWINCH) is
// only sent when cols/rows actually change. Without this, a burst of observer
// callbacks makes readline repaint the prompt down the screen.

const api = () => window.go.app.App;

// oled-black palette (mirrors configs/themes/oled-black.toml).
const THEME = {
  background: "#000000",
  foreground: "#d6d6d6",
  cursor: "#33b1ff",
  cursorAccent: "#000000",
  selectionBackground: "#0b2d45",
  black: "#0a0a0c", red: "#e06c75", green: "#09823a", yellow: "#d9a441",
  blue: "#33b1ff", magenta: "#c678dd", cyan: "#56b6c2", white: "#d6d6d6",
  brightBlack: "#4a4f58", brightRed: "#ff7b86", brightGreen: "#3fbf6a",
  brightYellow: "#f0c674", brightBlue: "#61c0ff", brightMagenta: "#d79aec",
  brightCyan: "#6fd3e0", brightWhite: "#ffffff",
};

function b64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

export function TerminalView({ sessionId, active }: { sessionId: string; active: boolean }) {
  const hostRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerm | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const lastSize = useRef({ cols: 0, rows: 0 });

  // fitAndSync: fit to the container, then push a resize ONLY when the
  // dimensions actually changed. Returns the fitted cols/rows.
  const fitAndSync = () => {
    const fit = fitRef.current, term = termRef.current;
    if (!fit || !term) return null;
    try { fit.fit(); } catch { return null; }
    const { cols, rows } = term;
    if (cols > 0 && rows > 0 &&
        (cols !== lastSize.current.cols || rows !== lastSize.current.rows)) {
      lastSize.current = { cols, rows };
      api().TermResize(sessionId, cols, rows).catch(() => {});
    }
    return { cols, rows };
  };

  useEffect(() => {
    const term = new XTerm({
      fontFamily: '"JetBrains Mono", ui-monospace, monospace',
      fontSize: 13,
      lineHeight: 1.1,
      theme: THEME,
      cursorBlink: true,
      scrollback: 5000,
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(hostRef.current!);
    termRef.current = term;
    fitRef.current = fit;

    // Initial fit before opening the PTY so it starts at the right size.
    // Record lastSize but don't send a resize yet (the terminal isn't open
    // on the Go side until OpenTerminal below).
    try { fit.fit(); } catch { /* hidden on mount: refit when active */ }
    lastSize.current = { cols: term.cols, rows: term.rows };

    const offData = window.runtime.EventsOn("f9:term:" + sessionId, (b64: string) => {
      term.write(b64ToBytes(b64));
    });
    const offClose = window.runtime.EventsOn("f9:termclose:" + sessionId, () => {
      term.write("\r\n\x1b[2m[session closed]\x1b[0m\r\n");
    });
    term.onData((d) => api().TermInput(sessionId, d));

    api().OpenTerminal(sessionId, term.cols, term.rows).catch(() => {});

    // Debounced observer: coalesce layout-settling bursts, then sync once.
    let timer: number | undefined;
    const ro = new ResizeObserver(() => {
      window.clearTimeout(timer);
      timer = window.setTimeout(fitAndSync, 60);
    });
    ro.observe(hostRef.current!);

    return () => {
      window.clearTimeout(timer);
      offData?.(); offClose?.(); ro.disconnect();
      api().CloseTerminal(sessionId).catch(() => {});
      term.dispose();
    };
  }, [sessionId]);

  // Becoming visible: refit once (deduped) and focus.
  useEffect(() => {
    if (active && fitRef.current && termRef.current) {
      requestAnimationFrame(() => {
        fitAndSync();
        try { termRef.current!.focus(); } catch { /* noop */ }
      });
    }
  }, [active]);

  return <div class="termhost" ref={hostRef} style={{ display: active ? "block" : "none" }} />;
}
