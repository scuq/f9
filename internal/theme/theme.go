// Package theme loads TOML color schemes covering the GUI palette, tree colors,
// the terminal 16-color palette and fonts. Builtin themes are embedded so the
// packaged binary is self-contained; user themes overlay from
// ~/.config/f9/themes/. See docs/phase-plan.md 03 and ADR-0003-adjacent notes.
package theme

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

//go:embed builtin
var builtinFS embed.FS

type UI struct {
	Bg         string `toml:"bg" json:"bg"`
	BgRaised   string `toml:"bg_raised" json:"bgRaised"`
	Fg         string `toml:"fg" json:"fg"`
	Accent     string `toml:"accent" json:"accent"`
	Border     string `toml:"border" json:"border"`
	FolderFg   string `toml:"folder_fg" json:"folderFg"`
	SelectedBg string `toml:"selected_bg" json:"selectedBg"`
	Danger     string `toml:"danger" json:"danger"`

	// Optional sidebar overrides. Empty => the session sidebar uses the main
	// colors above; set them to give the sidebar its own scheme (e.g. a light
	// sidebar on a dark-chrome theme).
	SidebarBg         string `toml:"sidebar_bg,omitempty" json:"sidebarBg"`
	SidebarBgRaised   string `toml:"sidebar_bg_raised,omitempty" json:"sidebarBgRaised"`
	SidebarFg         string `toml:"sidebar_fg,omitempty" json:"sidebarFg"`
	SidebarBorder     string `toml:"sidebar_border,omitempty" json:"sidebarBorder"`
	SidebarFolderFg   string `toml:"sidebar_folder_fg,omitempty" json:"sidebarFolderFg"`
	SidebarSelectedBg string `toml:"sidebar_selected_bg,omitempty" json:"sidebarSelectedBg"`
}

type Font struct {
	UI   string `toml:"ui" json:"ui"`
	Mono string `toml:"mono" json:"mono"`
	Size int    `toml:"size" json:"size"`
}

type ANSI struct {
	Black         string `toml:"black" json:"black"`
	Red           string `toml:"red" json:"red"`
	Green         string `toml:"green" json:"green"`
	Yellow        string `toml:"yellow" json:"yellow"`
	Blue          string `toml:"blue" json:"blue"`
	Magenta       string `toml:"magenta" json:"magenta"`
	Cyan          string `toml:"cyan" json:"cyan"`
	White         string `toml:"white" json:"white"`
	BrightBlack   string `toml:"bright_black" json:"brightBlack"`
	BrightRed     string `toml:"bright_red" json:"brightRed"`
	BrightGreen   string `toml:"bright_green" json:"brightGreen"`
	BrightYellow  string `toml:"bright_yellow" json:"brightYellow"`
	BrightBlue    string `toml:"bright_blue" json:"brightBlue"`
	BrightMagenta string `toml:"bright_magenta" json:"brightMagenta"`
	BrightCyan    string `toml:"bright_cyan" json:"brightCyan"`
	BrightWhite   string `toml:"bright_white" json:"brightWhite"`
}

type Terminal struct {
	Background   string `toml:"background" json:"background"`
	Foreground   string `toml:"foreground" json:"foreground"`
	Cursor       string `toml:"cursor" json:"cursor"`
	CursorAccent string `toml:"cursor_accent" json:"cursorAccent"`
	Selection    string `toml:"selection" json:"selection"`
	ANSI         ANSI   `toml:"ansi" json:"ansi"`
}

type Theme struct {
	Name     string   `toml:"name" json:"name"`
	UI       UI       `toml:"ui" json:"ui"`
	Font     Font     `toml:"font" json:"font"`
	Terminal Terminal `toml:"terminal" json:"terminal"`
}

var hexRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (t *Theme) validate() error {
	checks := map[string]string{
		"ui.bg": t.UI.Bg, "ui.fg": t.UI.Fg, "ui.accent": t.UI.Accent,
		"ui.border": t.UI.Border, "ui.selected_bg": t.UI.SelectedBg,
		"terminal.background": t.Terminal.Background, "terminal.foreground": t.Terminal.Foreground,
		"terminal.ansi.green": t.Terminal.ANSI.Green, "terminal.ansi.red": t.Terminal.ANSI.Red,
	}
	for field, v := range checks {
		if !hexRe.MatchString(v) {
			return fmt.Errorf("theme %q: %s = %q is not a #rrggbb color", t.Name, field, v)
		}
	}
	if t.Font.Size <= 0 {
		return fmt.Errorf("theme %q: font.size must be positive", t.Name)
	}
	return nil
}

func decode(data []byte) (*Theme, error) {
	var t Theme
	if err := toml.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	if t.Name == "" {
		return nil, fmt.Errorf("theme: missing name")
	}
	if err := t.validate(); err != nil {
		return nil, err
	}
	return &t, nil
}

// LoadAll returns embedded builtin themes overlaid with user themes from
// ~/.config/f9/themes/. Invalid theme files are skipped.
func LoadAll() map[string]*Theme {
	out := map[string]*Theme{}
	if entries, err := builtinFS.ReadDir("builtin"); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
				continue
			}
			if data, err := builtinFS.ReadFile("builtin/" + e.Name()); err == nil {
				if t, err := decode(data); err == nil {
					out[t.Name] = t
				}
			}
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		loadDirInto(filepath.Join(home, ".config", "f9", "themes"), out)
	}
	return out
}

func loadDirInto(dir string, out map[string]*Theme) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		if t, err := decode(data); err == nil {
			out[t.Name] = t
		}
	}
}
