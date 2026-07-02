// f9 — cross-platform SSH client. This binary is the phase-00 CLI smoke
// harness (list/connect/grep) and stays forever as the headless test tool.
// See docs/phase-plan.md.
package main

import (
	"fmt"
	"os"
)

const version = "0.0.0-phase00"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println("f9", version)
	case "list", "connect", "grep":
		fmt.Fprintf(os.Stderr, "f9 %s: phase 00 not implemented yet — see docs/phase-plan.md\n", os.Args[1])
		os.Exit(1)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: f9 <command>

commands:
  list                 list sessions from the store
  connect <session>    raw stdio attach (phase-00 smoke test)
  grep <session> <re>  grep a recorded scrollback buffer
  version              print version`)
}
