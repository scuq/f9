// Applies a ThemeData two ways from one source: CSS custom properties on
// <html> (inline style overrides the :root fallback, so switching is live) and
// the xterm ITheme (terminals subscribe via onThemeChange).

const DEFAULT: ThemeData = {
  name: "oled-black",
  ui: { bg: "#000000", bgRaised: "#0a0a0c", fg: "#d6d6d6", accent: "#33b1ff",
        border: "#1b1b1f", folderFg: "#8a8f98", selectedBg: "#0b2d45", danger: "#e06c75" },
  font: { ui: "Inter", mono: "JetBrains Mono", size: 13 },
  terminal: {
    background: "#000000", foreground: "#d6d6d6", cursor: "#33b1ff",
    cursorAccent: "#000000", selection: "#0b2d45",
    ansi: { black: "#0a0a0c", red: "#e06c75", green: "#09823a", yellow: "#d9a441",
            blue: "#33b1ff", magenta: "#c678dd", cyan: "#56b6c2", white: "#d6d6d6",
            brightBlack: "#4a4f58", brightRed: "#ff7b86", brightGreen: "#3fbf6a",
            brightYellow: "#f0c674", brightBlue: "#61c0ff", brightMagenta: "#d79aec",
            brightCyan: "#6fd3e0", brightWhite: "#ffffff" },
  },
};

let current: ThemeData = DEFAULT;
const subs = new Set<(t: ThemeData) => void>();

export function applyTheme(t: ThemeData) {
  current = t;
  const r = document.documentElement.style;
  r.setProperty("--bg", t.ui.bg);
  r.setProperty("--bg-raised", t.ui.bgRaised);
  r.setProperty("--fg", t.ui.fg);
  r.setProperty("--accent", t.ui.accent);
  r.setProperty("--border", t.ui.border);
  r.setProperty("--folder-fg", t.ui.folderFg);
  r.setProperty("--selected-bg", t.ui.selectedBg);
  r.setProperty("--danger", t.ui.danger);
  r.setProperty("--font-ui", `"${t.font.ui}", system-ui, -apple-system, "Segoe UI", sans-serif`);
  r.setProperty("--font-mono", `"${t.font.mono}", ui-monospace, "SFMono-Regular", Menlo, monospace`);
  r.setProperty("--font-size", `${t.font.size}px`);
  subs.forEach((f) => f(t));
}

export function currentTheme(): ThemeData { return current; }
export function onThemeChange(f: (t: ThemeData) => void): () => void {
  subs.add(f);
  return () => { subs.delete(f); };
}

export function xtermTheme(t: ThemeData) {
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
