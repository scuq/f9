package sshx

import "os"

// AgentKey describes a public key currently loaded in an ssh-agent.
type AgentKey struct {
	Comment     string `json:"comment"`
	Format      string `json:"format"`      // key type, e.g. ssh-ed25519
	Fingerprint string `json:"fingerprint"` // SHA256:...
}

// AgentEndpoint is one agent socket and what it reports.
type AgentEndpoint struct {
	Socket    string     `json:"socket"`
	Available bool       `json:"available"`
	Keys      []AgentKey `json:"keys"`
	Error     string     `json:"error"`
}

// resolveAgentSockets returns the sockets to try: the configured list if any,
// otherwise SSH_AUTH_SOCK, otherwise none. GUI apps frequently don't inherit a
// shell's exported SSH_AUTH_SOCK, so an explicit list is the reliable path.
func resolveAgentSockets(configured []string) []string {
	var out []string
	for _, s := range configured {
		if s != "" {
			out = append(out, s)
		}
	}
	if len(out) > 0 {
		return out
	}
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		return []string{sock}
	}
	return nil
}
