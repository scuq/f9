//go:build !gui

package app

// emitEvent is a no-op without the gui tag, except for an optional test hook
// (a.onEmit) used to drive prompt-routing tests.
func (a *App) emitEvent(event string, data interface{}) {
	if a.onEmit != nil {
		a.onEmit(event, data)
	}
}
