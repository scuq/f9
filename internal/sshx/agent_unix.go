//go:build !windows

package sshx

import (
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// agentSigners returns a lazy signer source backed by the ssh-agent socket,
// or nil if no agent is available. The agent connection stays open for the
// lifetime of the auth attempt (signing happens through it).
// TODO(00c follow-up): tie connection lifetime to the client, not the process.
func agentSigners() func() ([]ssh.Signer, error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil
	}
	return func() ([]ssh.Signer, error) {
		conn, err := net.Dial("unix", sock)
		if err != nil {
			return nil, err
		}
		return agent.NewClient(conn).Signers()
	}
}

// AgentAvailable reports whether an ssh-agent is reachable and its socket path.
func AgentAvailable() (bool, string) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return false, ""
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return false, sock
	}
	_ = conn.Close()
	return true, sock
}

// AgentKeys lists the keys currently loaded in the ssh-agent.
func AgentKeys() ([]AgentKey, error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, nil
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	keys, err := agent.NewClient(conn).List()
	if err != nil {
		return nil, err
	}
	out := make([]AgentKey, 0, len(keys))
	for _, k := range keys {
		out = append(out, AgentKey{
			Comment:     k.Comment,
			Format:      k.Format,
			Fingerprint: ssh.FingerprintSHA256(k),
		})
	}
	return out, nil
}
