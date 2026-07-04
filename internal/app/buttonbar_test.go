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

func TestGlobalBar(t *testing.T) {
	a, id, _ := setupConnectedTerminal(t)
	bar := buttonbar.Bar{Rows: []buttonbar.Row{{Buttons: []buttonbar.Button{
		{Label: "help", Action: buttonbar.Action{Kind: "url", Text: "https://x.example"}},
	}}}}
	if err := a.BarSave("", bar); err != nil {
		t.Fatal(err)
	}
	if got := a.GlobalBar(id); len(got.Rows) != 1 || got.Rows[0].Buttons[0].Label != "help" {
		t.Fatalf("GlobalBar(session) = %+v", got)
	}
	if got := a.GlobalBar(""); len(got.Rows) != 1 {
		t.Fatal("global bar should show with no session")
	}
}

func TestBarForSessionOSFilter(t *testing.T) {
	a, id, _ := setupConnectedTerminal(t)
	m, _ := a.st.Meta(id)
	m.SessionID = id
	m.DetectedOS = "nxos"
	if err := a.st.PutMeta(m); err != nil {
		t.Fatal(err)
	}
	s, _, _ := a.st.Resolve(id)
	bar := buttonbar.Bar{Rows: []buttonbar.Row{{Buttons: []buttonbar.Button{
		{Label: "any", Action: buttonbar.Action{Kind: "send", Text: "x"}},
		{Label: "ios", OS: "ios", Action: buttonbar.Action{Kind: "send", Text: "x"}},
		{Label: "nxos", OS: "nxos", Action: buttonbar.Action{Kind: "send", Text: "x"}},
	}}}}
	if err := a.BarSave(s.FolderID, bar); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range a.BarForSession(id).Rows {
		for _, b := range r.Buttons {
			got = append(got, b.Label)
		}
	}
	if len(got) != 2 || got[0] != "any" || got[1] != "nxos" {
		t.Fatalf("nxos-filtered bar = %v, want [any nxos]", got)
	}
}
