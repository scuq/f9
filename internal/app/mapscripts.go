package app

import "github.com/scuq/f9/internal/luamap"

// MapScriptList returns the global Lua map scripts, sorted by name.
func (a *App) MapScriptList() []luamap.Script { return a.maps.List() }

// MapScriptPut creates or replaces a named map script (parse-checked).
func (a *App) MapScriptPut(name, code string) error { return a.maps.Put(name, code) }

// MapScriptDelete removes a named map script.
func (a *App) MapScriptDelete(name string) error { return a.maps.Delete(name) }
