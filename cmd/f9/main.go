// f9 — cross-platform SSH client. This binary is the phase-00 CLI smoke
// harness and stays forever as the headless test tool. See docs/phase-plan.md.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/scuq/f9/internal/store"
)

const version = "0.0.1-phase00a"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println("f9", version)
	case "list":
		if err := cmdList(); err != nil {
			fmt.Fprintln(os.Stderr, "f9 list:", err)
			os.Exit(1)
		}
	case "connect", "grep":
		fmt.Fprintf(os.Stderr, "f9 %s: not implemented yet (phase 00b/00c) — see docs/phase-plan.md\n", os.Args[1])
		os.Exit(1)
	default:
		usage()
		os.Exit(2)
	}
}

func storeRoot() (string, error) {
	if p := os.Getenv("F9_STORE"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(home, ".config", "f9", "sessions"), nil
}

func cmdList() error {
	root, err := storeRoot()
	if err != nil {
		return err
	}
	st, err := store.Open(root)
	if err != nil {
		return err
	}
	sessions := st.Sessions()
	if len(sessions) == 0 {
		fmt.Printf("no sessions in %s (set F9_STORE to use another store)\n", root)
		return nil
	}
	for _, s := range sessions {
		target := s.Host
		if s.User != "" {
			target = s.User + "@" + target
		}
		if s.Port != 0 && s.Port != 22 {
			target = fmt.Sprintf("%s:%d", target, s.Port)
		}
		fmt.Printf("%-44s %-32s %s\n", st.FolderPath(s.FolderID)+"/"+s.Name, target, s.Proto)
	}
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: f9 <command>

commands:
  list                 list sessions from the store (F9_STORE overrides path)
  connect <session>    raw stdio attach (phase 00c)
  grep <session> <re>  grep a recorded scrollback buffer (phase 00b)
  version              print version`)
}
