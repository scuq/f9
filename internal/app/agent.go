package app

import "github.com/scuq/f9/internal/sshx"

// AgentStatus reports the ssh-agent endpoints (configured sockets, or
// SSH_AUTH_SOCK) and the keys each holds.
type AgentStatus struct {
	Endpoints []sshx.AgentEndpoint `json:"endpoints"`
}

// SSHAgentStatus returns the status of each configured agent socket. Safe to
// call any time; unreachable sockets are reported, not raised as errors.
func (a *App) SSHAgentStatus() AgentStatus {
	gs := a.Settings()
	eps := sshx.AgentEndpoints(gs.AgentSockets)
	if eps == nil {
		eps = []sshx.AgentEndpoint{}
	}
	return AgentStatus{Endpoints: eps}
}
