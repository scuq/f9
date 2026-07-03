package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/scuq/f9/internal/theme"
)

// Themes returns the available theme names, sorted.
func (a *App) Themes() []string {
	names := make([]string, 0, len(a.themes))
	for n := range a.themes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Theme returns one theme by name.
func (a *App) Theme(name string) (*theme.Theme, error) {
	t, ok := a.themes[name]
	if !ok {
		return nil, fmt.Errorf("app: unknown theme %q", name)
	}
	return t, nil
}

// CurrentTheme returns the selected theme name.
func (a *App) CurrentTheme() string { return a.themeName }

// SetTheme selects and persists a theme.
func (a *App) SetTheme(name string) error {
	if _, ok := a.themes[name]; !ok {
		return fmt.Errorf("app: unknown theme %q", name)
	}
	a.themeName = name
	return saveUITheme(name)
}

type uiSettings struct {
	Theme string `yaml:"theme"`
}

func uiSettingsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "f9", "ui.yaml")
}

func loadUITheme() string {
	p := uiSettingsPath()
	if p == "" {
		return ""
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	var s uiSettings
	if yaml.Unmarshal(data, &s) != nil {
		return ""
	}
	return s.Theme
}

func saveUITheme(name string) error {
	p := uiSettingsPath()
	if p == "" {
		return fmt.Errorf("app: no home dir for ui settings")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(uiSettings{Theme: name})
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

func initialThemeName(themes map[string]*theme.Theme) string {
	if n := loadUITheme(); n != "" {
		if _, ok := themes[n]; ok {
			return n
		}
	}
	if _, ok := themes["oled-black"]; ok {
		return "oled-black"
	}
	for n := range themes {
		return n
	}
	return ""
}
