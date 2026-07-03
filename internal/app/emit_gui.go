//go:build gui

package app

import "github.com/wailsapp/wails/v2/pkg/runtime"

// emitEvent forwards to the Wails runtime. Only compiled with the gui tag, so
// internal/app stays cgo/Wails-free for the cross-compile matrix.
func (a *App) emitEvent(event string, data interface{}) {
	if a.ctx == nil {
		return
	}
	if data == nil {
		runtime.EventsEmit(a.ctx, event)
		return
	}
	runtime.EventsEmit(a.ctx, event, data)
}
