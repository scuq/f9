package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// cliPrompter implements sshx.Prompter on the terminal. Secrets are read
// without echo and never persisted (ADR-0005).
type cliPrompter struct {
	in *bufio.Reader
}

func newCLIPrompter() *cliPrompter {
	return &cliPrompter{in: bufio.NewReader(os.Stdin)}
}

func (c *cliPrompter) Passphrase(keyPath string) (string, error) {
	fmt.Fprintf(os.Stderr, "Passphrase for %s: ", keyPath)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return string(b), err
}

func (c *cliPrompter) Password(user, host string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s@%s password: ", user, host)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return string(b), err
}

func (c *cliPrompter) KeyboardInteractive(name, instruction string, questions []string, echos []bool) ([]string, error) {
	if instruction != "" {
		fmt.Fprintln(os.Stderr, instruction)
	}
	answers := make([]string, len(questions))
	for i, q := range questions {
		fmt.Fprint(os.Stderr, q)
		if echos[i] {
			line, err := c.in.ReadString('\n')
			if err != nil {
				return nil, err
			}
			answers[i] = strings.TrimRight(line, "\r\n")
		} else {
			b, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(os.Stderr)
			if err != nil {
				return nil, err
			}
			answers[i] = string(b)
		}
	}
	return answers, nil
}

func (c *cliPrompter) ConfirmHostKey(host, fingerprint string) (bool, error) {
	fmt.Fprintf(os.Stderr, "Unknown host %s\nKey fingerprint: %s\nAccept and save? [y/N] ", host, fingerprint)
	line, err := c.in.ReadString('\n')
	if err != nil {
		return false, err
	}
	a := strings.ToLower(strings.TrimSpace(line))
	return a == "y" || a == "yes", nil
}
