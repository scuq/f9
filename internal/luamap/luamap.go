// Package luamap runs a user-provided Lua map(r) hook over import records:
// return the (possibly modified) record table to keep it, nil to drop it. The
// VM is sandboxed (base/table/string/math only; no os, io, or file loading)
// and bounded by the caller's context.
package luamap

import (
	"context"
	"fmt"

	lua "github.com/yuin/gopher-lua"

	"github.com/scuq/f9/internal/store"
)

// Apply compiles code (which must define map(r)) and runs it over recs.
// altUsers backs f9.alt_user(label). A per-record error aborts with the
// record's name in the message.
func Apply(ctx context.Context, code string, recs []store.ImportRecord, altUsers map[string]string) ([]store.ImportRecord, error) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()
	for _, lib := range []struct {
		name string
		fn   lua.LGFunction
	}{
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
		{lua.MathLibName, lua.OpenMath},
	} {
		if err := L.CallByParam(lua.P{Fn: L.NewFunction(lib.fn), NRet: 0, Protect: true}, lua.LString(lib.name)); err != nil {
			return nil, fmt.Errorf("luamap: open %s: %w", lib.name, err)
		}
	}
	// base includes filesystem escapes; remove them.
	L.SetGlobal("dofile", lua.LNil)
	L.SetGlobal("loadfile", lua.LNil)

	// f9 helper table: alt_user(label) -> username or nil.
	f9tbl := L.NewTable()
	f9tbl.RawSetString("alt_user", L.NewFunction(func(ls *lua.LState) int {
		if u, ok := altUsers[ls.CheckString(1)]; ok {
			ls.Push(lua.LString(u))
		} else {
			ls.Push(lua.LNil)
		}
		return 1
	}))
	L.SetGlobal("f9", f9tbl)
	L.SetContext(ctx)

	if err := L.DoString(code); err != nil {
		return nil, fmt.Errorf("luamap: load: %w", err)
	}
	mapFn := L.GetGlobal("map")
	if _, ok := mapFn.(*lua.LFunction); !ok {
		return nil, fmt.Errorf("luamap: script must define map(r)")
	}

	out := make([]store.ImportRecord, 0, len(recs))
	for i := range recs {
		if err := L.CallByParam(lua.P{Fn: mapFn, NRet: 1, Protect: true}, recordToLua(L, recs[i])); err != nil {
			return nil, fmt.Errorf("luamap: map(%q): %w", recs[i].Name, err)
		}
		ret := L.Get(-1)
		L.Pop(1)
		if ret == lua.LNil {
			continue
		}
		rt, ok := ret.(*lua.LTable)
		if !ok {
			return nil, fmt.Errorf("luamap: map(%q): return the record table or nil, got %s", recs[i].Name, ret.Type())
		}
		out = append(out, luaToRecord(rt, recs[i]))
	}
	return out, nil
}

func recordToLua(L *lua.LState, r store.ImportRecord) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("externalId", lua.LString(r.ExternalID))
	t.RawSetString("name", lua.LString(r.Name))
	t.RawSetString("host", lua.LString(r.Host))
	t.RawSetString("port", lua.LNumber(r.Port))
	t.RawSetString("user", lua.LString(r.User))
	t.RawSetString("proto", lua.LString(r.Proto))
	t.RawSetString("folder", lua.LString(r.Folder))
	tags := L.NewTable()
	for _, tg := range r.Tags {
		tags.Append(lua.LString(tg))
	}
	t.RawSetString("tags", tags)
	attrs := L.NewTable()
	for k, v := range r.Attrs {
		attrs.RawSetString(k, lua.LString(v))
	}
	t.RawSetString("attrs", attrs)
	if r.Raw != nil {
		t.RawSetString("raw", goToLua(L, map[string]interface{}(r.Raw)))
	} else {
		t.RawSetString("raw", L.NewTable())
	}
	return t
}

func luaToRecord(t *lua.LTable, base store.ImportRecord) store.ImportRecord {
	out := base
	if s, ok := t.RawGetString("name").(lua.LString); ok {
		out.Name = string(s)
	}
	if s, ok := t.RawGetString("host").(lua.LString); ok {
		out.Host = string(s)
	}
	if s, ok := t.RawGetString("user").(lua.LString); ok {
		out.User = string(s)
	}
	if s, ok := t.RawGetString("proto").(lua.LString); ok {
		out.Proto = string(s)
	}
	if s, ok := t.RawGetString("folder").(lua.LString); ok {
		out.Folder = string(s)
	}
	if n, ok := t.RawGetString("port").(lua.LNumber); ok {
		out.Port = int(n)
	}
	if tt, ok := t.RawGetString("tags").(*lua.LTable); ok {
		var tags []string
		tt.ForEach(func(_, v lua.LValue) {
			if s, ok := v.(lua.LString); ok {
				tags = append(tags, string(s))
			}
		})
		out.Tags = tags
	}
	return out
}

func goToLua(L *lua.LState, v interface{}) lua.LValue {
	switch t := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(t)
	case string:
		return lua.LString(t)
	case int:
		return lua.LNumber(t)
	case float64:
		return lua.LNumber(t)
	case []interface{}:
		tbl := L.NewTable()
		for _, e := range t {
			tbl.Append(goToLua(L, e))
		}
		return tbl
	case map[string]interface{}:
		tbl := L.NewTable()
		for k, e := range t {
			tbl.RawSetString(k, goToLua(L, e))
		}
		return tbl
	default:
		return lua.LString(fmt.Sprintf("%v", t))
	}
}
