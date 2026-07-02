//go:build windows

package sshx

import "golang.org/x/crypto/ssh"

// agentSigners on Windows: the OpenSSH agent listens on the named pipe
// \\.\pipe\openssh-ssh-agent, which requires go-winio to dial.
// TODO(00c follow-up): add go-winio-backed agent support. Until then key
// files and interactive auth work; agent auth is simply skipped.
func agentSigners() func() ([]ssh.Signer, error) { return nil }
