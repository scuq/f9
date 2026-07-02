// Package grep implements the "virtual grep" overlay semantics on top of
// scrollback.Buffer: grep-compatible flags (-v -i -A -B -E), chainable
// (pipe-style re-grep of results), streaming and cancelable. Phase 04.
package grep
