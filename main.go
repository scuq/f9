//go:build gui

// f9 GUI entry point (Wails v2). Built only with the gui tag so that plain
// `go build ./...` and the cross-compile matrix never need cgo/webkit:
//
//	make gui-dev / make gui-build
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/scuq/f9/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	a, err := app.New()
	if err != nil {
		log.Fatalf("f9-gui: %v", err)
	}
	err = wails.Run(&options.App{
		Title:            "f9",
		Width:            1440,
		Height:           900,
		MinWidth:         900,
		MinHeight:        600,
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        a.Startup,
		Bind:             []interface{}{a},
	})
	if err != nil {
		log.Fatalf("f9-gui: %v", err)
	}
}
