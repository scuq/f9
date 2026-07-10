package luamap

import (
	"fmt"
	"os"
	"sort"
	"sync"

	lua "github.com/yuin/gopher-lua"
	"gopkg.in/yaml.v3"
)

// Script is one named map script in the global library.
type Script struct {
	Name string `yaml:"name" json:"name"`
	Code string `yaml:"code" json:"code"`
}

// Library is the file-backed global script library (a single YAML file).
type Library struct {
	path string

	mu      sync.RWMutex
	scripts []Script
}

// OpenLibrary loads (creating lazily on first save) the library at path.
func OpenLibrary(path string) (*Library, error) {
	l := &Library{path: path}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return l, nil
		}
		return nil, fmt.Errorf("luamap: read library: %w", err)
	}
	if err := yaml.Unmarshal(b, &l.scripts); err != nil {
		return nil, fmt.Errorf("luamap: parse library: %w", err)
	}
	return l, nil
}

// List returns the scripts sorted by name.
func (l *Library) List() []Script {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := append([]Script(nil), l.scripts...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get returns the code of the named script.
func (l *Library) Get(name string) (string, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, s := range l.scripts {
		if s.Name == name {
			return s.Code, true
		}
	}
	return "", false
}

// Put creates or replaces a named script after a parse check (the script is
// compiled, not executed).
func (l *Library) Put(name, code string) error {
	if name == "" {
		return fmt.Errorf("luamap: script name required")
	}
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()
	if _, err := L.LoadString(code); err != nil {
		return fmt.Errorf("luamap: parse: %w", err)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.scripts {
		if l.scripts[i].Name == name {
			l.scripts[i].Code = code
			return l.save()
		}
	}
	l.scripts = append(l.scripts, Script{Name: name, Code: code})
	return l.save()
}

// Delete removes the named script.
func (l *Library) Delete(name string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.scripts {
		if l.scripts[i].Name == name {
			l.scripts = append(l.scripts[:i], l.scripts[i+1:]...)
			return l.save()
		}
	}
	return fmt.Errorf("luamap: script %q not found", name)
}

func (l *Library) save() error {
	b, err := yaml.Marshal(l.scripts)
	if err != nil {
		return fmt.Errorf("luamap: marshal library: %w", err)
	}
	if err := os.WriteFile(l.path, b, 0o600); err != nil {
		return fmt.Errorf("luamap: write library: %w", err)
	}
	return nil
}
