package theme

import "testing"

func TestLoadAllBuiltins(t *testing.T) {
	themes := LoadAll()
	ob, ok := themes["oled-black"]
	if !ok {
		t.Fatal("oled-black builtin missing")
	}
	if ob.UI.Bg != "#000000" {
		t.Fatalf("oled-black ui.bg = %q, want #000000", ob.UI.Bg)
	}
	if ob.Terminal.ANSI.Green != "#09823a" {
		t.Fatalf("oled-black ansi.green = %q", ob.Terminal.ANSI.Green)
	}
	if ob.Font.Mono != "JetBrains Mono" {
		t.Fatalf("oled-black font.mono = %q", ob.Font.Mono)
	}
	if _, ok := themes["gruvbox-dark"]; !ok {
		t.Fatal("gruvbox-dark builtin missing")
	}
}

func TestValidateRejectsBadHex(t *testing.T) {
	_, err := decode([]byte(`
name = "bad"
[ui]
bg = "not-a-color"
fg = "#ffffff"
accent = "#ffffff"
border = "#ffffff"
selected_bg = "#ffffff"
[font]
size = 13
[terminal]
background = "#000000"
foreground = "#ffffff"
[terminal.ansi]
green = "#00ff00"
red = "#ff0000"
`))
	if err == nil {
		t.Fatal("expected validation error for bad hex")
	}
}
