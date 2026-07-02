//go:build windows

package main

import (
	"time"

	"golang.org/x/term"

	"github.com/scuq/f9/internal/sshx"
)

// watchResize polls for terminal size changes (no SIGWINCH on Windows).
func watchResize(fd int, s sshx.Session) {
	lastC, lastR := 0, 0
	for {
		time.Sleep(time.Second)
		cols, rows, err := term.GetSize(fd)
		if err != nil {
			return
		}
		if cols != lastC || rows != lastR {
			lastC, lastR = cols, rows
			_ = s.Resize(cols, rows)
		}
	}
}
