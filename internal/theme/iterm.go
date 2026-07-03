package theme

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

type plistColor struct{ R, G, B float64 }

// parseItermColors walks the .itermcolors plist and returns the color dicts
// (e.g. "Ansi 2 Color", "Background Color") as fractional RGB. It reads only
// the structure iTerm2 emits: a top dict whose values are color dicts of
// "<component> Component" -> <real 0..1>.
func parseItermColors(data []byte) (map[string]plistColor, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	colors := map[string]plistColor{}

	depth := 0
	topKey := ""
	comp := ""
	target := "" // where the next CharData goes: topkey | comp | real
	inColor := false
	var cur plistColor

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch t.Name.Local {
			case "key":
				if depth == 3 {
					target = "topkey"
				} else if depth == 4 && inColor {
					target = "comp"
				}
			case "dict":
				if depth == 3 {
					inColor = true
					cur = plistColor{}
				}
			case "real", "integer":
				if depth == 4 && inColor {
					target = "real"
				}
			}
		case xml.CharData:
			s := strings.TrimSpace(string(t))
			if s == "" {
				break
			}
			switch target {
			case "topkey":
				topKey = s
			case "comp":
				comp = s
			case "real":
				v, _ := strconv.ParseFloat(s, 64)
				switch comp {
				case "Red Component":
					cur.R = v
				case "Green Component":
					cur.G = v
				case "Blue Component":
					cur.B = v
				}
			}
			target = ""
		case xml.EndElement:
			if t.Name.Local == "dict" && depth == 3 && inColor {
				colors[topKey] = cur
				inColor = false
			}
			depth--
		}
	}
	return colors, nil
}

func clamp8(f float64) int {
	v := int(math.Round(f * 255))
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

func toHex(c plistColor) string {
	return fmt.Sprintf("#%02x%02x%02x", clamp8(c.R), clamp8(c.G), clamp8(c.B))
}

func parseHex(h string) (int, int, int) {
	var r, g, b int
	fmt.Sscanf(h, "#%02x%02x%02x", &r, &g, &b)
	return r, g, b
}

// lerpHex linearly interpolates a toward b by f in [0,1].
func lerpHex(a, b string, f float64) string {
	ar, ag, ab := parseHex(a)
	br, bg, bb := parseHex(b)
	li := func(x, y int) int { return int(math.Round(float64(x) + (float64(y)-float64(x))*f)) }
	return fmt.Sprintf("#%02x%02x%02x", li(ar, br), li(ag, bg), li(ab, bb))
}

func orHex(v, fallback string) string {
	if hexRe.MatchString(v) {
		return v
	}
	return fallback
}

var nameClean = regexp.MustCompile(`[^a-z0-9]+`)

func sanitizeThemeName(s string) string {
	s = strings.ToLower(s)
	s = nameClean.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "imported"
	}
	return s
}

// ImportITerm parses an .itermcolors file into a Theme. The terminal palette
// maps directly; the GUI palette is derived (iTerm2 has none); fonts default.
func ImportITerm(path string) (*Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	colors, err := parseItermColors(data)
	if err != nil {
		return nil, fmt.Errorf("theme: parse %s: %w", filepath.Base(path), err)
	}
	hex := func(k string) string {
		c, ok := colors[k]
		if !ok {
			return ""
		}
		return toHex(c)
	}

	name := sanitizeThemeName(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	t := &Theme{Name: name, Font: Font{UI: "Inter", Mono: "JetBrains Mono", Size: 13}}
	t.Terminal.ANSI = ANSI{
		Black: hex("Ansi 0 Color"), Red: hex("Ansi 1 Color"), Green: hex("Ansi 2 Color"), Yellow: hex("Ansi 3 Color"),
		Blue: hex("Ansi 4 Color"), Magenta: hex("Ansi 5 Color"), Cyan: hex("Ansi 6 Color"), White: hex("Ansi 7 Color"),
		BrightBlack: hex("Ansi 8 Color"), BrightRed: hex("Ansi 9 Color"), BrightGreen: hex("Ansi 10 Color"), BrightYellow: hex("Ansi 11 Color"),
		BrightBlue: hex("Ansi 12 Color"), BrightMagenta: hex("Ansi 13 Color"), BrightCyan: hex("Ansi 14 Color"), BrightWhite: hex("Ansi 15 Color"),
	}
	bg := orHex(hex("Background Color"), "#000000")
	fg := orHex(hex("Foreground Color"), "#d6d6d6")
	t.Terminal.Background = bg
	t.Terminal.Foreground = fg
	t.Terminal.Cursor = orHex(hex("Cursor Color"), orHex(t.Terminal.ANSI.Blue, fg))
	t.Terminal.CursorAccent = orHex(hex("Cursor Text Color"), bg)
	t.Terminal.Selection = orHex(hex("Selection Color"), lerpHex(bg, fg, 0.20))

	t.UI = UI{
		Bg:         bg,
		BgRaised:   lerpHex(bg, fg, 0.06),
		Fg:         fg,
		Accent:     orHex(t.Terminal.ANSI.Blue, t.Terminal.Cursor),
		Border:     lerpHex(bg, fg, 0.14),
		FolderFg:   orHex(t.Terminal.ANSI.BrightBlack, lerpHex(bg, fg, 0.45)),
		SelectedBg: t.Terminal.Selection,
		Danger:     orHex(t.Terminal.ANSI.Red, "#e06c75"),
	}
	if err := t.validate(); err != nil {
		return nil, err
	}
	return t, nil
}

// SaveUser writes a theme as TOML into dir, returning the file path.
func SaveUser(t *Theme, dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("theme: empty theme dir")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, t.Name+".toml")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := toml.NewEncoder(f).Encode(t); err != nil {
		return "", err
	}
	return path, nil
}
