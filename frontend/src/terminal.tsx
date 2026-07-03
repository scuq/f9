import { useEffect, useRef } from "preact/hooks";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { currentTheme, xtermTheme, onThemeChange } from "./theme";

const api = () => window.go.app.App;

function b64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

const monoStack = (m: string) => `"${m}", ui-monospace, monospace`;

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
    if (cols > 0 && rows > 0 && (cols !== lastSize.current.cols || rows !== lastSize.current.rows)) {
      lastSize.current = { cols, rows };
      api().TermResize(termId, cols, rows).catch(() => {});
    }
  };

  useEffect(() => {
    const th = currentTheme();
    const term = new XTerm({
      fontFamily: monoStack(th.font.mono),
      fontSize: th.font.size,
      lineHeight: 1.1,
      theme: xtermTheme(th),
      cursorBlink: true,
      scrollback: 5000,
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(hostRef.current!);
    termRef.current = term;
    fitRef.current = fit;
    try { fit.fit(); } catch { /* hidden on mount */ }
    lastSize.current = { cols: term.cols, rows: term.rows };

    const offData = window.runtime.EventsOn("f9:term:" + termId, (b64: string) => term.write(b64ToBytes(b64)));
    term.onData((d) => api().TermInput(termId, d));
    api().OpenTerminal(termId, sessionId, term.cols, term.rows).catch(() => {});

    const offTheme = onThemeChange((t) => {
      term.options.theme = xtermTheme(t);
      term.options.fontFamily = monoStack(t.font.mono);
      term.options.fontSize = t.font.size;
      requestAnimationFrame(fitAndSync);
    });

    let timer: number | undefined;
    const ro = new ResizeObserver(() => { window.clearTimeout(timer); timer = window.setTimeout(fitAndSync, 60); });
    ro.observe(hostRef.current!);

    return () => {
      window.clearTimeout(timer);
      offData?.(); offTheme(); ro.disconnect();
      api().CloseTerminal(termId).catch(() => {});
      term.dispose();
    };
  }, [termId]);

  useEffect(() => {
    if (active && fitRef.current && termRef.current) {
      requestAnimationFrame(() => { fitAndSync(); try { termRef.current!.focus(); } catch { /* noop */ } });
    }
  }, [active]);

  return <div class="termhost" ref={hostRef} style={{ display: active ? "block" : "none" }} />;
}
