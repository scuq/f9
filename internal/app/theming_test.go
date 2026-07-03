package app

import "testing"

func TestThemeBindings(t *testing.T) {
	a, _, _ := newTestApp(t)
	names := a.Themes()
	if len(names) < 2 {
		t.Fatalf("themes = %v, want at least oled-black + gruvbox-dark", names)
	}
	th, err := a.Theme("oled-black")
	if err != nil || th.UI.Bg != "#000000" {
		t.Fatalf("Theme(oled-black) = %+v, %v", th, err)
	}
	if _, err := a.Theme("does-not-exist"); err == nil {
		t.Fatal("expected error for unknown theme")
	}
	if a.CurrentTheme() == "" {
		t.Fatal("CurrentTheme empty")
	}
	if err := a.SetTheme("gruvbox-dark"); err != nil {
		t.Fatal(err)
	}
	if a.CurrentTheme() != "gruvbox-dark" {
		t.Fatalf("CurrentTheme = %q after SetTheme", a.CurrentTheme())
	}
	if err := a.SetTheme("nope"); err == nil {
		t.Fatal("expected error setting unknown theme")
	}
}
