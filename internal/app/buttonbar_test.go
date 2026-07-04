package app

import (
	"testing"

	"github.com/scuq/f9/internal/buttonbar"
)

func TestBarForSession(t *testing.T) {
	a, id, _ := setupConnectedTerminal(t)
	s, _, err := a.st.Resolve(id)
	if err != nil {
		t.Fatal(err)
	}
	bar := buttonbar.Bar{Rows: []buttonbar.Row{{Buttons: []buttonbar.Button{
		{Label: "save", Action: buttonbar.Action{Kind: "send", Text: "{{ save_cmd }}"}},
	}}}}
	if err := a.BarSave(s.FolderID, bar); err != nil {
		t.Fatal(err)
	}
	got := a.BarForSession(id)
	if len(got.Rows) != 1 || got.Rows[0].Buttons[0].Label != "save" {
		t.Fatalf("BarForSession = %+v", got)
	}
}

func TestLaunchAppRejectsEmpty(t *testing.T) {
	a, _, _ := setupConnectedTerminal(t)
	if err := a.LaunchApp(nil); err == nil {
		t.Fatal("expected error for empty argv")
	}
	if err := a.LaunchApp([]string{"  "}); err == nil {
		t.Fatal("expected error for blank program")
	}
}

func TestOpenURLRejectsBadScheme(t *testing.T) {
	a, _, _ := setupConnectedTerminal(t)
	if err := a.OpenURL("file:///etc/passwd"); err == nil {
		t.Fatal("expected rejection of file scheme")
	}
	if err := a.OpenURL("not a url"); err == nil {
		t.Fatal("expected rejection of scheme-less string")
	}
}
