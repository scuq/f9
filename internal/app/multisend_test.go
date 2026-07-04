package app

import (
	"testing"

	"github.com/scuq/f9/internal/multisend"
	"github.com/scuq/f9/internal/osdetect"
)

func linuxTuned(t *testing.T, a *App, id string) {
	m, _ := a.st.Meta(id)
	m.SessionID = id
	m.DetectedOS = "linux"
	if err := a.st.PutMeta(m); err != nil {
		t.Fatal(err)
	}
	a.tunings["linux"] = osdetect.Tuning{PromptRegex: `[\w.\-@:~]+[#>$]\s*$`, ErrorRegex: `% [A-Z]`}
}

func TestMultiSendPreview(t *testing.T) {
	a, id, _ := setupConnectedTerminal(t)
	if err := a.OpenTerminal("T1", id, 80, 24); err != nil {
		t.Fatal(err)
	}
	if err := a.VarsPut(VarsScopeDTO{SessionID: id}, "save_cmd", "write memory", "all"); err != nil {
		t.Fatal(err)
	}
	prev := a.MultiSendPreview([]string{"T1"}, "{{ save_cmd }}")
	if len(prev) != 1 || prev[0].Line != "write memory" {
		t.Fatalf("preview = %+v", prev)
	}
	prev2 := a.MultiSendPreview([]string{"T1"}, "{{ vlan_id }}")
	if len(prev2[0].Unresolved) != 1 || prev2[0].Unresolved[0] != "vlan_id" {
		t.Fatalf("unresolved = %+v", prev2)
	}
}

func TestMultiSendSingleTarget(t *testing.T) {
	a, id, fs := setupConnectedTerminal(t)
	if err := a.OpenTerminal("T1", id, 80, 24); err != nil {
		t.Fatal(err)
	}
	linuxTuned(t, a, id)

	var states []string
	a.onEmit = func(event string, data interface{}) {
		if event == "f9:multisend" {
			if r, ok := data.(multisend.Result); ok {
				states = append(states, string(r.State))
			}
		}
	}
	if err := a.MultiSendStart([]string{"T1"}, "show clock", nil, false, 5000); err != nil {
		t.Fatal(err)
	}
	if got := fs.stdin.String(); got != "show clock\r" {
		t.Fatalf("stdin = %q, want %q", got, "show clock\r")
	}
	fs.onData([]byte("show clock\r\nSun Jul  5 00:42 UTC 2026\r\nlyrael:~$ "))
	if len(states) == 0 || states[len(states)-1] != "ok" {
		t.Fatalf("states = %v, want end ok", states)
	}
	a.MultiSendCancel()
}

func TestMultiSendBusyAndCancel(t *testing.T) {
	a, id, _ := setupConnectedTerminal(t)
	if err := a.OpenTerminal("T1", id, 80, 24); err != nil {
		t.Fatal(err)
	}
	linuxTuned(t, a, id)
	if err := a.MultiSendStart([]string{"T1"}, "x", nil, false, 5000); err != nil {
		t.Fatal(err)
	}
	if err := a.MultiSendStart([]string{"T1"}, "y", nil, false, 5000); err == nil {
		t.Fatal("expected busy error")
	}
	a.MultiSendCancel()
	if err := a.MultiSendStart([]string{"T1"}, "z", nil, false, 5000); err != nil {
		t.Fatalf("should start after cancel: %v", err)
	}
	a.MultiSendCancel()
}
