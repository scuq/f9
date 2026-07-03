//go:build !gui

package main

import "fmt"

func main() {
	fmt.Println("f9-gui stub: the GUI is built via Wails with the gui tag —")
	fmt.Println("  make gui-dev    # live-reload development window")
	fmt.Println("  make gui-build  # production binary (build/bin/f9-gui)")
	fmt.Println("The CLI lives in cmd/f9.")
}
