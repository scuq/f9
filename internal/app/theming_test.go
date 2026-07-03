package app

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleIterm = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Ansi 1 Color</key>
	<dict><key>Red Component</key><real>1.0</real><key>Green Component</key><real>0.0</real><key>Blue Component</key><real>0.0</real></dict>
	<key>Ansi 2 Color</key>
	<dict><key>Red Component</key><real>0.0</real><key>Green Component</key><real>1.0</real><key>Blue Component</key><real>0.0</real></dict>
	<key>Ansi 4 Color</key>
	<dict><key>Red Component</key><real>0.0</real><key>Green Component</key><real>0.0</real><key>Blue Component</key><real>1.0</real></dict>
	<key>Background Color</key>
	<dict><key>Red Component</key><real>0.0</real><key>Green Component</key><real>0.0</real><key>Blue Component</key><real>0.0</real></dict>
	<key>Foreground Color</key>
	<dict><key>Red Component</key><real>1.0</real><key>Green Component</key><real>1.0</real><key>Blue Component</key><real>1.0</real></dict>
	<key>Selection Color</key>
	<dict><key>Red Component</key><real>0.04</real><key>Green Component</key><real>0.18</real><key>Blue Component</key><real>0.27</real></dict>
</dict>
</plist>`

func TestThemeBindings(t *testing.T) {
	a, _, _ := newTestApp(t)
	if len(a.Themes()) < 2 {
		t.Fatalf("themes = %v, want >= 2", a.Themes())
	}
	th, err := a.Theme("oled-black")
	if err != nil || th.UI.Bg != "#0a0b0d" {
		t.Fatalf("Theme(oled-black) = %+v, %v", th, err)
	}
	if _, err := a.Theme("nope"); err == nil {
		t.Fatal("expected error for unknown theme")
	}
}

func TestSettingsRoundtrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("F9_STORE", t.TempDir())
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	s := a.Settings()
	if s.Zoom != 1 {
		t.Fatalf("default zoom = %v, want 1", s.Zoom)
	}
	s.Theme = "gruvbox-dark"
	s.Zoom = 1.25
	s.FontTermSize = 15
	s.FontMono = "Fira Code"
	if err := a.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	got := a.Settings()
	if got.Theme != "gruvbox-dark" || got.Zoom != 1.25 || got.FontTermSize != 15 || got.FontMono != "Fira Code" {
		t.Fatalf("settings = %+v", got)
	}
	if a.CurrentTheme() != "gruvbox-dark" {
		t.Fatalf("themeName = %q", a.CurrentTheme())
	}
}

func TestImportITermFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("F9_STORE", t.TempDir())
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "MyScheme.itermcolors")
	if err := os.WriteFile(p, []byte(sampleIterm), 0o600); err != nil {
		t.Fatal(err)
	}
	name, err := a.importITermFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if name != "myscheme" {
		t.Fatalf("name = %q, want myscheme", name)
	}
	if th, err := a.Theme("myscheme"); err != nil || th.Terminal.ANSI.Green != "#00ff00" {
		t.Fatalf("imported theme wrong: %+v, %v", th, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "f9", "themes", "myscheme.toml")); err != nil {
		t.Fatalf("theme not persisted: %v", err)
	}
}
