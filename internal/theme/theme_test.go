package theme

import "testing"

func TestLoadAllBuiltins(t *testing.T) {
	themes := LoadAll()
	ob, ok := themes["oled-black"]
	if !ok {
		t.Fatal("oled-black builtin missing")
	}
	if ob.UI.Bg != "#0a0b0d" {
		t.Fatalf("oled-black ui.bg = %q, want #0a0b0d", ob.UI.Bg)
	}
	if ob.Terminal.ANSI.Green != "#2ea043" {
		t.Fatalf("oled-black ansi.green = %q", ob.Terminal.ANSI.Green)
	}
	if ob.Font.Mono != "JetBrains Mono" {
		t.Fatalf("oled-black font.mono = %q", ob.Font.Mono)
	}
	if _, ok := themes["gruvbox-dark"]; !ok {
		t.Fatal("gruvbox-dark builtin missing")
	}
	gb, ok := themes["green-on-black"]
	if !ok {
		t.Fatal("green-on-black builtin missing")
	}
	if gb.Terminal.Foreground != "#33ff66" {
		t.Fatalf("green-on-black terminal.foreground = %q", gb.Terminal.Foreground)
	}
	bl, ok := themes["becklight"]
	if !ok {
		t.Fatal("becklight builtin missing")
	}
	if bl.Terminal.Background != "#ffffff" || bl.Terminal.ANSI.Blue != "#3465a4" {
		t.Fatalf("becklight colors off: bg=%q blue=%q", bl.Terminal.Background, bl.Terminal.ANSI.Blue)
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
