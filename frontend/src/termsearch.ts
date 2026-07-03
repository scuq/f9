// Lets a terminal's Ctrl/Cmd+F handler ask the app to open the scrollback
// search (the ⌕ panel). Find = the Go-side grep over full history; there is no
// separate xterm-viewport search.

const findRequesters = new Set<(termId: string) => void>();

export function requestFind(termId: string) {
  findRequesters.forEach((f) => f(termId));
}

export function onFindRequested(cb: (termId: string) => void): () => void {
  findRequesters.add(cb);
  return () => { findRequesters.delete(cb); };
}
