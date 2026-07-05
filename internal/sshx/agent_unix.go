//go:build !windows

package sshx

import (
	"net"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// agentSignersFor returns a lazy signer source for a specific agent socket.
// An unreachable socket errors at auth time (the method is skipped), not a panic.
func agentSignersFor(sock string) func() ([]ssh.Signer, error) {
	return func() ([]ssh.Signer, error) {
		conn, err := net.Dial("unix", sock)
		if err != nil {
			return nil, err
		}
		return agent.NewClient(conn).Signers()
	}
}

// AgentEndpoints reports each resolved agent socket and the keys it holds.
func AgentEndpoints(configured []string) []AgentEndpoint {
	sockets := resolveAgentSockets(configured)
	out := make([]AgentEndpoint, 0, len(sockets))
	for _, sock := range sockets {
		ep := AgentEndpoint{Socket: sock, Keys: []AgentKey{}}
		conn, err := net.Dial("unix", sock)
		if err != nil {
			ep.Error = err.Error()
			out = append(out, ep)
			continue
		}
		keys, err := agent.NewClient(conn).List()
		_ = conn.Close()
		if err != nil {
			ep.Error = err.Error()
			out = append(out, ep)
			continue
		}
		ep.Available = true
		for _, k := range keys {
			ep.Keys = append(ep.Keys, AgentKey{
				Comment:     k.Comment,
				Format:      k.Format,
				Fingerprint: ssh.FingerprintSHA256(k),
			})
		}
		out = append(out, ep)
	}
	return out
}
