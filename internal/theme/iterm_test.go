package theme

import "testing"

const sampleIterm = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Ansi 0 Color</key>
	<dict><key>Red Component</key><real>0.04</real><key>Green Component</key><real>0.04</real><key>Blue Component</key><real>0.05</real></dict>
	<key>Ansi 1 Color</key>
	<dict><key>Red Component</key><real>1.0</real><key>Green Component</key><real>0.0</real><key>Blue Component</key><real>0.0</real></dict>
	<key>Ansi 2 Color</key>
	<dict><key>Red Component</key><real>0.0</real><key>Green Component</key><real>1.0</real><key>Blue Component</key><real>0.0</real></dict>
	<key>Ansi 4 Color</key>
	<dict><key>Red Component</key><real>0.0</real><key>Green Component</key><real>0.0</real><key>Blue Component</key><real>1.0</real></dict>
	<key>Ansi 8 Color</key>
	<dict><key>Red Component</key><real>0.29</real><key>Green Component</key><real>0.31</real><key>Blue Component</key><real>0.34</real></dict>
	<key>Background Color</key>
	<dict><key>Red Component</key><real>0.0</real><key>Green Component</key><real>0.0</real><key>Blue Component</key><real>0.0</real></dict>
	<key>Foreground Color</key>
	<dict><key>Red Component</key><real>1.0</real><key>Green Component</key><real>1.0</real><key>Blue Component</key><real>1.0</real></dict>
	<key>Cursor Color</key>
	<dict><key>Red Component</key><real>0.2</real><key>Green Component</key><real>0.69</real><key>Blue Component</key><real>1.0</real></dict>
	<key>Selection Color</key>
	<dict><key>Red Component</key><real>0.04</real><key>Green Component</key><real>0.18</real><key>Blue Component</key><real>0.27</real></dict>
</dict>
</plist>`

func TestImportITermMapping(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/My Cool Scheme.itermcolors"
	if err := writeFile(path, sampleIterm); err != nil {
		t.Fatal(err)
	}
	th, err := ImportITerm(path)
	if err != nil {
		t.Fatal(err)
	}
	if th.Name != "my-cool-scheme" {
		t.Fatalf("name = %q, want my-cool-scheme", th.Name)
	}
	if th.Terminal.ANSI.Green != "#00ff00" {
		t.Fatalf("ansi.green = %q", th.Terminal.ANSI.Green)
	}
	if th.Terminal.ANSI.Red != "#ff0000" {
		t.Fatalf("ansi.red = %q", th.Terminal.ANSI.Red)
	}
	if th.Terminal.Background != "#000000" || th.Terminal.Foreground != "#ffffff" {
		t.Fatalf("bg/fg = %q / %q", th.Terminal.Background, th.Terminal.Foreground)
	}
	if th.UI.Accent != "#0000ff" {
		t.Fatalf("ui.accent = %q, want #0000ff (Ansi 4)", th.UI.Accent)
	}
	if th.UI.Fg != "#ffffff" {
		t.Fatalf("ui.fg = %q", th.UI.Fg)
	}
	// derived middle-grey border between #000 and #fff at 0.14
	if th.UI.Border == th.UI.Bg {
		t.Fatalf("border not derived: %q", th.UI.Border)
	}
}

func writeFile(path, content string) error {
	return osWriteFile(path, []byte(content))
}
