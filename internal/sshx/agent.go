package sshx

// AgentKey describes a public key currently loaded in the ssh-agent.
type AgentKey struct {
	Comment     string `json:"comment"`
	Format      string `json:"format"`      // key type, e.g. ssh-ed25519
	Fingerprint string `json:"fingerprint"` // SHA256:...
}
