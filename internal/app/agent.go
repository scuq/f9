package app

import "github.com/scuq/f9/internal/sshx"

// AgentStatus reports ssh-agent availability and its loaded keys.
type AgentStatus struct {
	Available bool            `json:"available"`
	Socket    string          `json:"socket"`
	Keys      []sshx.AgentKey `json:"keys"`
	Error     string          `json:"error"`
}

// SSHAgentStatus returns the current ssh-agent status and loaded keys. Safe to
// call any time; a missing/unreachable agent yields Available=false, not an error.
func (a *App) SSHAgentStatus() AgentStatus {
	avail, sock := sshx.AgentAvailable()
	st := AgentStatus{Available: avail, Socket: sock, Keys: []sshx.AgentKey{}}
	if !avail {
		return st
	}
	keys, err := sshx.AgentKeys()
	if err != nil {
		st.Error = err.Error()
		return st
	}
	if keys != nil {
		st.Keys = keys
	}
	return st
}
