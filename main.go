//go:build gui

// f9 GUI entry point (Wails v2). Built only with the gui tag so that plain
// `go build ./...` and the cross-compile matrix never need cgo/webkit:
//
//	make gui-dev / make gui-build
package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"github.com/scuq/f9/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

// webviewDataPath picks where WebView2 stores its user data on Windows. The
// default %APPDATA%\\f9-gui.exe path is often blocked by locked-down AV/EDR, so
// f9 (a portable single-exe) defaults to a folder next to the executable,
// overridable via the F9_WEBVIEW_DATA environment variable. Ignored elsewhere.
func webviewDataPath() string {
	if p := os.Getenv("F9_WEBVIEW_DATA"); p != "" {
		return p
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "f9-webview2")
	}
	return ""
}

func main() {
	// WebKitGTK renders blurry through the DMABUF path on virtio/VM GPUs;
	// force the SHM renderer for crisp 1:1 output. Ignored on macOS/Windows.
	os.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "1")

	a, err := app.New()
	if err != nil {
		log.Fatalf("f9-gui: %v", err)
	}
	err = wails.Run(&options.App{
		Title:            "f9",
		Frameless:        true,
		Width:            1440,
		Height:           900,
		MinWidth:         900,
		MinHeight:        600,
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        a.Startup,
		Bind:             []interface{}{a},
		Linux: &linux.Options{
			Icon: appIcon,
		},
		Windows: &windows.Options{
			WebviewUserDataPath: webviewDataPath(),
		},
	})
	if err != nil {
		log.Fatalf("f9-gui: %v", err)
	}
}
