// One source of truth applied two ways: CSS custom properties on <html>
// (GUI/tree) and the xterm ITheme (terminals). Font family/size and zoom are
// user overrides that layer over the active theme's values.

const DEFAULT_THEME: ThemeData = {
  name: "oled-black",
  ui: { bg: "#0a0b0d", bgRaised: "#14161a", fg: "#c6c9cf", accent: "#4db2f0",
        border: "#24272d", folderFg: "#7d828b", selectedBg: "#123049", danger: "#e06c75" },
  font: { ui: "Inter", mono: "JetBrains Mono", size: 13 },
  terminal: {
    background: "#0a0b0d", foreground: "#c6c9cf", cursor: "#4db2f0",
    cursorAccent: "#0a0b0d", selection: "#123049",
    ansi: { black: "#14161a", red: "#e06c75", green: "#2ea043", yellow: "#d9a441",
            blue: "#4db2f0", magenta: "#c678dd", cyan: "#56b6c2", white: "#c6c9cf",
            brightBlack: "#5c626b", brightRed: "#ff7b86", brightGreen: "#3fbf6a",
            brightYellow: "#f0c674", brightBlue: "#61c0ff", brightMagenta: "#d79aec",
            brightCyan: "#6fd3e0", brightWhite: "#ffffff" },
  },
};

type Overrides = { zoom: number; fontUI: string; fontMono: string; fontUISize: number; fontTermSize: number };
type TermConfig = { theme: any; fontFamily: string; fontSize: number };

let curTheme: ThemeData = DEFAULT_THEME;
let ov: Overrides = { zoom: 1, fontUI: "", fontMono: "", fontUISize: 0, fontTermSize: 0 };
const subs = new Set<(c: TermConfig) => void>();

const uiFont = () => ov.fontUI || curTheme.font.ui;
const monoFont = () => ov.fontMono || curTheme.font.mono;
const uiSize = () => ov.fontUISize || curTheme.font.size;
const termSize = () => ov.fontTermSize || curTheme.font.size;
const monoStack = (m: string) => `"${m}", ui-monospace, "SFMono-Regular", Menlo, Consolas, monospace`;

function xtermTheme(t: ThemeData) {
  const a = t.terminal.ansi;
  return {
    background: t.terminal.background, foreground: t.terminal.foreground,
    cursor: t.terminal.cursor, cursorAccent: t.terminal.cursorAccent,
    selectionBackground: t.terminal.selection,
    black: a.black, red: a.red, green: a.green, yellow: a.yellow,
    blue: a.blue, magenta: a.magenta, cyan: a.cyan, white: a.white,
    brightBlack: a.brightBlack, brightRed: a.brightRed, brightGreen: a.brightGreen,
    brightYellow: a.brightYellow, brightBlue: a.brightBlue, brightMagenta: a.brightMagenta,
    brightCyan: a.brightCyan, brightWhite: a.brightWhite,
  };
}

function termConfig(): TermConfig {
  return { theme: xtermTheme(curTheme), fontFamily: monoStack(monoFont()), fontSize: termSize() };
}

function applyAll() {
  const r = document.documentElement.style;
  const u = curTheme.ui;
  r.setProperty("--bg", u.bg);
  r.setProperty("--bg-raised", u.bgRaised);
  r.setProperty("--fg", u.fg);
  r.setProperty("--accent", u.accent);
  r.setProperty("--border", u.border);
  r.setProperty("--folder-fg", u.folderFg);
  r.setProperty("--selected-bg", u.selectedBg);
  r.setProperty("--danger", u.danger);
  r.setProperty("--font-ui", `"${uiFont()}", system-ui, -apple-system, "Segoe UI", sans-serif`);
  r.setProperty("--font-mono", monoStack(monoFont()));
  r.setProperty("--font-size", `${uiSize()}px`);
  (r as any).zoom = String(ov.zoom || 1);
  const c = termConfig();
  subs.forEach((f) => f(c));
}

export function setTheme(t: ThemeData) { curTheme = t; applyAll(); }
export function setSettings(s: Partial<Overrides>) { ov = { ...ov, ...s }; applyAll(); }
export function currentTermConfig(): TermConfig { return termConfig(); }
export function onTermConfig(f: (c: TermConfig) => void): () => void {
  subs.add(f);
  return () => { subs.delete(f); };
}
