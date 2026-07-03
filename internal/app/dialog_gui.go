//go:build gui

package app

import "github.com/wailsapp/wails/v2/pkg/runtime"

// ImportITermTheme opens a native file picker for an .itermcolors file and
// imports it. Returns the imported theme name, or "" if cancelled.
func (a *App) ImportITermTheme() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Import iTerm2 color scheme",
		Filters: []runtime.FileFilter{
			{DisplayName: "iTerm2 Colors (*.itermcolors)", Pattern: "*.itermcolors"},
		},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil
	}
	return a.importITermFile(path)
}
