//go:build windows

package sshx

import (
	"errors"

	"golang.org/x/crypto/ssh"
)

// errAgentUnsupported: the Windows OpenSSH agent uses a named pipe, which needs
// go-winio to dial. Until that lands, agent auth is unavailable on Windows.
var errAgentUnsupported = errors.New("ssh-agent not supported on Windows")

func agentSignersFor(_ string) func() ([]ssh.Signer, error) {
	return func() ([]ssh.Signer, error) { return nil, errAgentUnsupported }
}

// AgentEndpoints reports configured sockets as unavailable on Windows.
func AgentEndpoints(configured []string) []AgentEndpoint {
	sockets := resolveAgentSockets(configured)
	out := make([]AgentEndpoint, 0, len(sockets))
	for _, sock := range sockets {
		out = append(out, AgentEndpoint{Socket: sock, Keys: []AgentKey{}, Error: "ssh-agent not supported on Windows yet"})
	}
	return out
}
