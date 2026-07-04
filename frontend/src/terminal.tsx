import { useEffect, useRef } from "preact/hooks";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { currentTermConfig, onTermConfig } from "./theme";
import { requestFind, requestPicker, pickerIsEnabled } from "./termsearch";

const api = () => window.go.app.App;

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
    if (cols > 0 && rows > 0 && (cols !== lastSize.current.cols || rows !== lastSize.current.rows)) {
      lastSize.current = { cols, rows };
      api().TermResize(termId, cols, rows).catch(() => {});
    }
  };

  useEffect(() => {
    const c = currentTermConfig();
    const term = new XTerm({
      fontFamily: c.fontFamily,
      fontSize: c.fontSize,
      lineHeight: 1.1,
      theme: c.theme,
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

    // Intercept Ctrl/Cmd+F before xterm forwards it to the shell; open the
    // scrollback search panel instead.
    term.attachCustomKeyEventHandler((e) => {
      if (e.type === "keydown" && (e.ctrlKey || e.metaKey) && !e.altKey && (e.key === "f" || e.key === "F")) {
        e.preventDefault();
        requestFind(termId);
        return false;
      }
      if (e.type === "keydown" && (e.ctrlKey || e.metaKey) && !e.altKey && (e.key === "p" || e.key === "P") && pickerIsEnabled()) {
        e.preventDefault();
        requestPicker(termId);
        return false;
      }
      return true;
    });

    const offCfg = onTermConfig((cfg) => {
      term.options.theme = cfg.theme;
      term.options.fontFamily = cfg.fontFamily;
      term.options.fontSize = cfg.fontSize;
      requestAnimationFrame(fitAndSync);
    });

    let timer: number | undefined;
    const ro = new ResizeObserver(() => { window.clearTimeout(timer); timer = window.setTimeout(fitAndSync, 60); });
    ro.observe(hostRef.current!);

    return () => {
      window.clearTimeout(timer);
      offData?.(); offCfg(); ro.disconnect();
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
