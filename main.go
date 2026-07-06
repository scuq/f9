//go:build gui

// f9 GUI entry point (Wails v2). Built only with the gui tag so that plain
// `go build ./...` and the cross-compile matrix never need cgo/webkit:
//
//	make gui-dev / make gui-build
package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

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

// wantsIconInstall reports whether the CLI asked to install the Linux desktop
// icon + launcher (--install-icons, or its --install-images alias).
func wantsIconInstall() bool {
	for _, a := range os.Args[1:] {
		if a == "--install-icons" || a == "--install-images" {
			return true
		}
	}
	return false
}

// installDesktopIcons writes the embedded app icon into the freedesktop hicolor
// theme and a .desktop launcher under $XDG_DATA_HOME (default ~/.local/share),
// so f9 appears in the application menu. Per-user; no root required. Linux only.
func installDesktopIcons() error {
	if runtime.GOOS != "linux" {
		fmt.Println("f9-gui: --install-icons is only supported on Linux")
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dataDir = filepath.Join(home, ".local", "share")
	}

	iconDir := filepath.Join(dataDir, "icons", "hicolor", "256x256", "apps")
	if err := os.MkdirAll(iconDir, 0o755); err != nil {
		return err
	}
	iconPath := filepath.Join(iconDir, "f9.png")
	if err := os.WriteFile(iconPath, appIcon, 0o644); err != nil {
		return err
	}

	appsDir := filepath.Join(dataDir, "applications")
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		return err
	}
	desktop := "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=f9\n" +
		"Comment=SSH client\n" +
		"Exec=" + exe + " %U\n" +
		"Icon=f9\n" +
		"Terminal=false\n" +
		"Categories=Network;Utility;\n" +
		"StartupWMClass=f9-gui\n"
	desktopPath := filepath.Join(appsDir, "f9.desktop")
	if err := os.WriteFile(desktopPath, []byte(desktop), 0o644); err != nil {
		return err
	}

	// Best-effort cache refresh; harmless if the tools are absent.
	_ = exec.Command("gtk-update-icon-cache", "-q", "-t", "-f", filepath.Join(dataDir, "icons", "hicolor")).Run()
	_ = exec.Command("update-desktop-database", appsDir).Run()

	fmt.Println("f9-gui: installed icon     ->", iconPath)
	fmt.Println("f9-gui: installed launcher ->", desktopPath)
	return nil
}

func main() {
	if wantsIconInstall() {
		if err := installDesktopIcons(); err != nil {
			log.Fatalf("f9-gui: install icons: %v", err)
		}
		return
	}

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
