//go:build !gui

package app

import "errors"

// ImportITermTheme requires the Wails runtime file dialog (gui build).
func (a *App) ImportITermTheme() (string, error) {
	return "", errors.New("app: file dialog unavailable without the gui build")
}
