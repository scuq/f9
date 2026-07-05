package app

import "testing"

func TestSSHAgentStatusNoAgent(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	a, _, _ := newTestApp(t)
	st := a.SSHAgentStatus()
	if st.Available {
		t.Fatal("expected no agent with empty SSH_AUTH_SOCK")
	}
	if st.Keys == nil {
		t.Fatal("Keys should be a non-nil empty slice for JSON")
	}
}
