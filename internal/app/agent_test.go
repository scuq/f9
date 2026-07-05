package app

import "testing"

func TestSSHAgentStatusNoAgent(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	a, _, _ := newTestApp(t)
	st := a.SSHAgentStatus()
	if st.Endpoints == nil {
		t.Fatal("Endpoints should be a non-nil (empty) slice for JSON")
	}
	if len(st.Endpoints) != 0 {
		t.Fatalf("expected 0 endpoints with no agent, got %d", len(st.Endpoints))
	}
}
