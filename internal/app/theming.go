package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"

	"github.com/scuq/f9/internal/theme"
)

// UISettings is the persisted UI preference set (~/.config/f9/ui.yaml). Font
// fields empty/zero mean "use the theme's value".
type UISettings struct {
	Theme         string  `yaml:"theme" json:"theme"`
	Zoom          float64 `yaml:"zoom,omitempty" json:"zoom"`
	FontUI        string  `yaml:"font_ui,omitempty" json:"fontUI"`
	FontMono      string  `yaml:"font_mono,omitempty" json:"fontMono"`
	FontUISize    int     `yaml:"font_ui_size,omitempty" json:"fontUISize"`
	FontTermSize  int     `yaml:"font_term_size,omitempty" json:"fontTermSize"`
	ShowGlobalBar bool    `yaml:"show_global_bar,omitempty" json:"showGlobalBar"`
	ShowFolderBar bool    `yaml:"show_folder_bar,omitempty" json:"showFolderBar"`
	ShowTemplates bool    `yaml:"show_templates,omitempty" json:"showTemplates"`
	ShowSnippets  bool    `yaml:"show_snippets,omitempty" json:"showSnippets"`
	BarVertical   bool    `yaml:"bar_vertical,omitempty" json:"barVertical"`
	BarUnpinned   bool    `yaml:"bar_unpinned,omitempty" json:"barUnpinned"`
}

func (a *App) Themes() []string {
	a.themeMu.RLock()
	defer a.themeMu.RUnlock()
	names := make([]string, 0, len(a.themes))
	for n := range a.themes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (a *App) Theme(name string) (*theme.Theme, error) {
	a.themeMu.RLock()
	defer a.themeMu.RUnlock()
	t, ok := a.themes[name]
	if !ok {
		return nil, fmt.Errorf("app: unknown theme %q", name)
	}
	return t, nil
}

func (a *App) CurrentTheme() string {
	a.themeMu.RLock()
	defer a.themeMu.RUnlock()
	return a.themeName
}

// Settings returns the persisted UI settings with sane defaults filled in.
func (a *App) Settings() UISettings {
	s := loadUISettings()
	if s.Theme == "" {
		s.Theme = a.CurrentTheme()
	}
	if s.Zoom == 0 {
		s.Zoom = 1
	}
	return s
}

// SaveSettings persists the UI settings and syncs the selected theme name.
func (a *App) SaveSettings(s UISettings) error {
	if s.Zoom == 0 {
		s.Zoom = 1
	}
	a.themeMu.Lock()
	if _, ok := a.themes[s.Theme]; ok {
		a.themeName = s.Theme
	}
	a.themeMu.Unlock()
	return saveUISettings(s)
}

// importITermFile imports an .itermcolors file, writes it as a user theme,
// reloads, and returns the new theme name (testable; gui dialog wraps it).
func (a *App) importITermFile(path string) (string, error) {
	t, err := theme.ImportITerm(path)
	if err != nil {
		return "", err
	}
	if _, err := theme.SaveUser(t, userThemeDir()); err != nil {
		return "", err
	}
	themes := theme.LoadAll()
	a.themeMu.Lock()
	a.themes = themes
	a.themeMu.Unlock()
	return t.Name, nil
}

func userThemeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "f9", "themes")
}

// startThemeWatcher live-reloads user themes on file changes (GUI only).
func (a *App) startThemeWatcher() {
	dir := userThemeDir()
	if dir == "" {
		return
	}
	_ = os.MkdirAll(dir, 0o700)
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	if err := w.Add(dir); err != nil {
		w.Close()
		return
	}
	go func() {
		defer w.Close()
		var timer *time.Timer
		reload := func() {
			themes := theme.LoadAll()
			a.themeMu.Lock()
			a.themes = themes
			a.themeMu.Unlock()
			a.emitEvent("f9:themes", nil)
		}
		for {
			select {
			case _, ok := <-w.Events:
				if !ok {
					return
				}
				if timer != nil {
					timer.Stop()
				}
				timer = time.AfterFunc(150*time.Millisecond, reload)
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()
}

func uiSettingsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "f9", "ui.yaml")
}

func loadUISettings() UISettings {
	p := uiSettingsPath()
	if p == "" {
		return UISettings{}
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return UISettings{}
	}
	var s UISettings
	_ = yaml.Unmarshal(data, &s)
	return s
}

func saveUISettings(s UISettings) error {
	p := uiSettingsPath()
	if p == "" {
		return fmt.Errorf("app: no home dir for ui settings")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

func initialThemeName(themes map[string]*theme.Theme) string {
	if n := loadUISettings().Theme; n != "" {
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
