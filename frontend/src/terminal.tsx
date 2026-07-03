import { useEffect, useRef } from "preact/hooks";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";

// Terminal keyed by a frontend-generated termId (N terminals per session).
// DOM renderer for sharpness on the WebKitGTK SHM path. Resize is debounced
// and deduped so readline never repaints the prompt down the screen.

const api = () => window.go.app.App;

const THEME = {
  background: "#000000", foreground: "#d6d6d6", cursor: "#33b1ff",
  cursorAccent: "#000000", selectionBackground: "#0b2d45",
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

export function TerminalView(
  { termId, sessionId, active }: { termId: string; sessionId: string; active: boolean },
) {
  const hostRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerm | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const lastSize = useRef({ cols: 0, rows: 0 });

  const fitAndSync = () => {
    const fit = fitRef.current, term = termRef.current;
    if (!fit || !term) return;
    try { fit.fit(); } catch { return; }
    const { cols, rows } = term;
    if (cols > 0 && rows > 0 &&
        (cols !== lastSize.current.cols || rows !== lastSize.current.rows)) {
      lastSize.current = { cols, rows };
      api().TermResize(termId, cols, rows).catch(() => {});
    }
  };

  useEffect(() => {
    const term = new XTerm({
      fontFamily: '"JetBrains Mono", ui-monospace, monospace',
      fontSize: 13, lineHeight: 1.1, theme: THEME,
      cursorBlink: true, scrollback: 5000,
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(hostRef.current!);
    termRef.current = term;
    fitRef.current = fit;
    try { fit.fit(); } catch { /* hidden on mount */ }
    lastSize.current = { cols: term.cols, rows: term.rows };

    const offData = window.runtime.EventsOn("f9:term:" + termId, (b64: string) => {
      term.write(b64ToBytes(b64));
    });
    term.onData((d) => api().TermInput(termId, d));
    api().OpenTerminal(termId, sessionId, term.cols, term.rows).catch(() => {});

    let timer: number | undefined;
    const ro = new ResizeObserver(() => {
      window.clearTimeout(timer);
      timer = window.setTimeout(fitAndSync, 60);
    });
    ro.observe(hostRef.current!);

    return () => {
      window.clearTimeout(timer);
      offData?.(); ro.disconnect();
      api().CloseTerminal(termId).catch(() => {});
      term.dispose();
    };
  }, [termId]);

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
