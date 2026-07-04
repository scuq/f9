package app

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"

	"github.com/scuq/f9/internal/buttonbar"
)

// BarForSession returns the effective button bar for a session's folder.
func (a *App) BarForSession(sessionID string) buttonbar.Bar {
	s, _, err := a.st.Resolve(sessionID)
	if err != nil {
		return buttonbar.Bar{}
	}
	fam := string(a.sessionFamily(sessionID))
	return a.bars.ResolveFolder(s.FolderID).FilterOS(fam)
}

// GlobalBar returns the always-visible global bar, OS-filtered for a session
// (sessionID "" = no active session, so undetected).
func (a *App) GlobalBar(sessionID string) buttonbar.Bar {
	fam := ""
	if sessionID != "" {
		fam = string(a.sessionFamily(sessionID))
	}
	g, _ := a.bars.Get("")
	return g.FilterOS(fam)
}

// BarResolved returns the effective bar for a folder (inherited/override).
func (a *App) BarResolved(folderID string) buttonbar.Bar { return a.bars.Resolve(folderID) }

// BarRaw returns a folder's own bar for editing, or nil if it inherits.
func (a *App) BarRaw(folderID string) *buttonbar.Bar {
	if b, ok := a.bars.Get(folderID); ok {
		return &b
	}
	return nil
}

func (a *App) BarSave(folderID string, bar buttonbar.Bar) error { return a.bars.Save(folderID, bar) }
func (a *App) BarDelete(folderID string) error                  { return a.bars.Delete(folderID) }
func (a *App) BarExport(folderID string) (string, error)        { return a.bars.Export(folderID) }
func (a *App) BarImport(folderID, yamlText string) error        { return a.bars.Import(folderID, yamlText) }

// LaunchApp runs a local program with the given argv. No shell is involved, so
// there is no shell-injection surface; args[0] is the program, args[1:] its
// arguments verbatim.
func (a *App) LaunchApp(args []string) error {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("app: launch requires a program")
	}
	cmd := exec.Command(args[0], args[1:]...)
	return cmd.Start()
}

var urlSchemeOK = map[string]bool{"http": true, "https": true, "ssh": true, "mailto": true, "ftp": true, "ftps": true}

// OpenURL opens a URL in the platform's default handler. The scheme must be in
// the allowlist so a button can't launch arbitrary local handlers.
func (a *App) OpenURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return fmt.Errorf("app: invalid url")
	}
	if !urlSchemeOK[strings.ToLower(u.Scheme)] {
		return fmt.Errorf("app: url scheme %q not allowed", u.Scheme)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", raw)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", raw)
	default:
		cmd = exec.Command("xdg-open", raw)
	}
	return cmd.Start()
}
