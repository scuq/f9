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

// Ctrl/Cmd+P opens the snippet picker. pickerEnabled gates the terminal
// intercept so Ctrl+P still reaches the shell when the feature is off.
const pickerRequesters = new Set<(termId: string) => void>();
let pickerEnabled = false;

export function requestPicker(termId: string) {
  pickerRequesters.forEach((f) => f(termId));
}

export function onPickerRequested(cb: (termId: string) => void): () => void {
  pickerRequesters.add(cb);
  return () => { pickerRequesters.delete(cb); };
}

export function setPickerEnabled(v: boolean) { pickerEnabled = v; }
export function pickerIsEnabled(): boolean { return pickerEnabled; }
