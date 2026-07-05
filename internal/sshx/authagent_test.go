//go:build !windows

package sshx

import "testing"

// TestBuildAuthAgentToggle checks that useAgent=false drops the agent method.
// agentSigners() only checks SSH_AUTH_SOCK (no dial), so a fake path suffices.
func TestBuildAuthAgentToggle(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/tmp/f9-fake-agent.sock")
	withAgent := buildAuth("u", "h", nil, true, nil)
	without := buildAuth("u", "h", nil, false, nil)
	if len(withAgent) != len(without)+1 {
		t.Fatalf("agent method not toggled: with=%d without=%d", len(withAgent), len(without))
	}
}
