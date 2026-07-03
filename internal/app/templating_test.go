package app

import (
	"strings"
	"testing"
)

func TestSendToTerminal(t *testing.T) {
	a, id, fs := setupConnectedTerminal(t)
	if err := a.OpenTerminal("T1", id, 80, 24); err != nil {
		t.Fatal(err)
	}
	if err := a.SendToTerminal("T1", "show version\n", 0, false); err != nil {
		t.Fatal(err)
	}
	if got := fs.stdin.String(); got != "show version\r" {
		t.Fatalf("stdin = %q, want %q", got, "show version\r")
	}
}

func TestSendToTerminalBracketed(t *testing.T) {
	a, id, fs := setupConnectedTerminal(t)
	if err := a.OpenTerminal("T1", id, 80, 24); err != nil {
		t.Fatal(err)
	}
	if err := a.SendToTerminal("T1", "conf t\nend", 0, true); err != nil {
		t.Fatal(err)
	}
	got := fs.stdin.String()
	if !strings.HasPrefix(got, "\x1b[200~") || !strings.HasSuffix(got, "\x1b[201~") || !strings.Contains(got, "conf t\nend") {
		t.Fatalf("bracketed stdin = %q", got)
	}
}

func TestSendTemplateResolvesVars(t *testing.T) {
	a, id, fs := setupConnectedTerminal(t)
	if err := a.OpenTerminal("T1", id, 80, 24); err != nil {
		t.Fatal(err)
	}
	if err := a.VarsPut(VarsScopeDTO{SessionID: id}, "hostname", "sw1", "all"); err != nil {
		t.Fatal(err)
	}
	if err := a.SendTemplate("T1", "hostname {{ hostname }} vlan {{ vlan_id }}", map[string]string{"vlan_id": "222"}, 0, false); err != nil {
		t.Fatal(err)
	}
	if got := fs.stdin.String(); got != "hostname sw1 vlan 222\r" {
		t.Fatalf("stdin = %q, want %q", got, "hostname sw1 vlan 222\r")
	}
}

func TestTemplateUnresolved(t *testing.T) {
	a, id, _ := setupConnectedTerminal(t)
	if err := a.VarsPut(VarsScopeDTO{SessionID: id}, "a", "1", "all"); err != nil {
		t.Fatal(err)
	}
	u, err := a.TemplateUnresolved(id, "{{ a }} {{ b }} {% if c %}x{% endif %}")
	if err != nil {
		t.Fatal(err)
	}
	if len(u) != 2 || u[0] != "b" || u[1] != "c" {
		t.Fatalf("unresolved = %v, want [b c]", u)
	}
}

func TestVarsPutRejectsSecret(t *testing.T) {
	a, id, _ := setupConnectedTerminal(t)
	if err := a.VarsPut(VarsScopeDTO{SessionID: id}, "api_token", "x", "all"); err == nil {
		t.Fatal("expected secret rejection via binding")
	}
}

func TestSendTemplateOSScoped(t *testing.T) {
	a, id, fs := setupConnectedTerminal(t)
	if err := a.OpenTerminal("T1", id, 80, 24); err != nil {
		t.Fatal(err)
	}
	m, _ := a.st.Meta(id)
	m.SessionID = id
	m.DetectedOS = "nxos"
	if err := a.st.PutMeta(m); err != nil {
		t.Fatal(err)
	}
	scope := VarsScopeDTO{SessionID: id}
	if err := a.VarsPut(scope, "save_cmd", "write memory", "all"); err != nil {
		t.Fatal(err)
	}
	if err := a.VarsPut(scope, "save_cmd", "copy run start", "nxos"); err != nil {
		t.Fatal(err)
	}
	if err := a.SendTemplate("T1", "{{ save_cmd }}", nil, 0, false); err != nil {
		t.Fatal(err)
	}
	if got := fs.stdin.String(); got != "copy run start\r" {
		t.Fatalf("stdin = %q, want %q", got, "copy run start\r")
	}
}
