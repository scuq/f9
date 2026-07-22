import { useEffect, useRef, useState } from "preact/hooks";
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
  { termId, sessionId, active, disconnected, onReconnect, confirmPaste }: { termId: string; sessionId: string; active: boolean; disconnected?: boolean; onReconnect?: () => void; confirmPaste?: boolean },
) {
  const hostRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerm | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const lastSize = useRef({ cols: 0, rows: 0 });
  const primaryRef = useRef("");
  const wasDisc = useRef(false);
  const onReconnectRef = useRef(onReconnect);
  onReconnectRef.current = onReconnect;
  const confirmRef = useRef(!!confirmPaste);
  confirmRef.current = !!confirmPaste;
  const [pendingPaste, setPendingPaste] = useState<string | null>(null);
  // routePaste sends text to the terminal, detouring multi-line pastes
  // through the review overlay when enabled.
  const routePaste = (text: string) => {
    if (!text) return;
    if (confirmRef.current && text.includes("\n")) {
      setPendingPaste(text);
      return;
    }
    termRef.current?.paste(text);
  };
  const routePasteRef = useRef(routePaste);
  routePasteRef.current = routePaste;

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
    api().OpenTerminal(termId, sessionId, term.cols, term.rows).catch((e) => {
      // A failed attach used to leave a live tab over a black rectangle with
      // the connection row still reading "connected". Show why instead.
      const raw = e && typeof e === "object" && "message" in (e as object) ? (e as { message?: string }).message : e;
      const msg = String(raw ?? "unknown error").replace(/[\r\n]+/g, " ");
      term.write("\r\n\x1b[31mf9: could not open terminal: " + msg + "\x1b[0m\r\n");
      term.write("\x1b[90mthe SSH connection itself is still up \u2014 close this tab and open a new terminal to retry\x1b[0m\r\n");
    });

    // Intercept Ctrl/Cmd+F before xterm forwards it to the shell; open the
    // scrollback search panel instead.
    term.attachCustomKeyEventHandler((e) => {
      // Enter on a disconnected terminal: reconnect the session in this tab.
      if (e.type === "keydown" && e.key === "Enter" && wasDisc.current) {
        e.preventDefault();
        onReconnectRef.current?.();
        return false;
      }
      // Ctrl/Cmd+Shift+C: copy the selection to the clipboard.
      if (e.type === "keydown" && (e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === "c" || e.key === "C")) {
        const sel = term.getSelection();
        if (sel) window.runtime.ClipboardSetText?.(sel);
        e.preventDefault();
        return false;
      }
      // Ctrl/Cmd+Shift+V: paste the clipboard into the terminal.
      if (e.type === "keydown" && (e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === "v" || e.key === "V")) {
        e.preventDefault();
        const p = window.runtime.ClipboardGetText?.();
        if (p) p.then((t) => { if (t) routePasteRef.current(t); }).catch(() => {});
        return false;
      }
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

    // Linux-style primary selection: remember the current selection and paste
    // it on middle-click.
    const primaryHost = hostRef.current!;
    const offSel = term.onSelectionChange(() => {
      const sel = term.getSelection();
      if (sel) primaryRef.current = sel;
    });
    // Middle-click pastes the terminal selection (Linux primary style). The
    // WebView also runs its own native middle-click paste; we can't cancel the
    // mousedown default, but the native paste still fires a DOM `paste` event —
    // suppress that so the content isn't pasted twice. Our term.paste() below
    // does not dispatch a DOM paste event, so it is unaffected.
    let suppressNativePaste = false;
    let suppressTimer: number | undefined;
    const onMiddlePaste = (e: MouseEvent) => {
      if (e.button === 1) {
        e.preventDefault();
        suppressNativePaste = true;
        window.clearTimeout(suppressTimer);
        suppressTimer = window.setTimeout(() => { suppressNativePaste = false; }, 200);
        const prim = primaryRef.current;
        if (prim) routePasteRef.current(prim);
      }
    };
    const onNativePaste = (e: ClipboardEvent) => {
      if (suppressNativePaste) {
        e.preventDefault();
        e.stopImmediatePropagation();
        suppressNativePaste = false;
        window.clearTimeout(suppressTimer);
      }
    };
    primaryHost.addEventListener("mousedown", onMiddlePaste, true);
    primaryHost.addEventListener("paste", onNativePaste, true);

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
      window.clearTimeout(suppressTimer);
      offSel.dispose();
      primaryHost.removeEventListener("mousedown", onMiddlePaste, true);
      primaryHost.removeEventListener("paste", onNativePaste, true);
      api().CloseTerminal(termId).catch(() => {});
      term.dispose();
    };
  }, [termId]);

  useEffect(() => {
    if (active && fitRef.current && termRef.current) {
      requestAnimationFrame(() => { fitAndSync(); try { termRef.current!.focus(); } catch { /* noop */ } });
      // second refit after the surrounding chrome (status bar, button strips)
      // has settled, so the bottom row is never left clipped by a late layout.
      const late = window.setTimeout(fitAndSync, 150);
      return () => window.clearTimeout(late);
    }
  }, [active]);

  // Once the session is gone, freeze the terminal: stop the cursor blink and
  // ignore input (except Enter, handled above as reconnect). Scrollback stays
  // intact for reading the last output.
  useEffect(() => {
    const term = termRef.current;
    if (!term || !disconnected || wasDisc.current) return;
    wasDisc.current = true;
    term.options.cursorBlink = false;
    term.options.disableStdin = true;
  }, [disconnected]);

  const commitPaste = () => {
    const t = pendingPaste;
    setPendingPaste(null);
    if (t) termRef.current?.paste(t);
    try { termRef.current?.focus(); } catch { /* noop */ }
  };
  return (
    <div class="termwrap" style={{ display: active ? "block" : "none" }}>
      <div class="termhost" ref={hostRef} />
      {pendingPaste !== null && (
        <div class="pastebox">
          <div class="pastebox-head">review paste ({pendingPaste.split("\n").length} lines)</div>
          <textarea
            ref={(el) => { if (el) el.focus(); }}
            value={pendingPaste}
            onInput={(e) => setPendingPaste((e.target as HTMLTextAreaElement).value)}
            onKeyDown={(e) => {
              if (e.key === "Escape") { e.preventDefault(); setPendingPaste(null); try { termRef.current?.focus(); } catch { /* noop */ } }
              if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) { e.preventDefault(); commitPaste(); }
            }}
          />
          <div class="pastebox-actions">
            <span class="pastebox-hint">{"Esc cancels \u00b7 Ctrl+Enter pastes"}</span>
            <button onClick={() => { setPendingPaste(null); try { termRef.current?.focus(); } catch { /* noop */ } }}>cancel</button>
            <button onClick={commitPaste}>paste</button>
          </div>
        </div>
      )}
    </div>
  );
}
