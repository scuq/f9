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
