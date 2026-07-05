//go:build !windows

package sshx

import "testing"

// TestBuildAuthAgentSockets: each socket adds one PublicKeysCallback; p=nil adds
// no password/keyboard-interactive, so counts differ only by the agent methods.
func TestBuildAuthAgentSockets(t *testing.T) {
	none := buildAuth("u", "h", nil, nil, nil)
	one := buildAuth("u", "h", nil, []string{"/tmp/a.sock"}, nil)
	two := buildAuth("u", "h", nil, []string{"/tmp/a.sock", "/tmp/b.sock"}, nil)
	if len(one) != len(none)+1 || len(two) != len(none)+2 {
		t.Fatalf("agent methods: none=%d one=%d two=%d", len(none), len(one), len(two))
	}
}
