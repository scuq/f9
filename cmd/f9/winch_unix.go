//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"

	"github.com/scuq/f9/internal/sshx"
)

// watchResize propagates terminal size changes to the remote PTY (SIGWINCH).
func watchResize(fd int, s sshx.Session) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	defer signal.Stop(ch)
	for range ch {
		if cols, rows, err := term.GetSize(fd); err == nil {
			_ = s.Resize(cols, rows)
		}
	}
}
