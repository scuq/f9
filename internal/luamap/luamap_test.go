package luamap

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/scuq/f9/internal/store"
)

func TestApplyMapRenameDropRaw(t *testing.T) {
	recs := []store.ImportRecord{
		{ExternalID: "1", Name: "a", Host: "a", Attrs: map[string]string{"role": "core"},
			Raw: map[string]interface{}{"preferedIpAddress": "10.9.9.9"}},
		{ExternalID: "2", Name: "b", Host: "b", Attrs: map[string]string{"role": "edge"}},
	}
	code := `
function map(r)
  if r.attrs.role == "edge" then return nil end
  if r.raw.preferedIpAddress then r.host = r.raw.preferedIpAddress end
  r.name = r.name .. "_" .. r.host
  r.folder = "00-KAG/SERVER"
  return r
end`
	out, err := Apply(context.Background(), code, recs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Name != "a_10.9.9.9" || out[0].Host != "10.9.9.9" || out[0].Folder != "00-KAG/SERVER" {
		t.Fatalf("out = %+v", out)
	}
}

func TestApplyAltUser(t *testing.T) {
	code := `
function map(r)
  assert(f9.alt_user("missing") == nil, "missing label must be nil")
  r.user = f9.alt_user("linux") or "fallback"
  return r
end`
	out, err := Apply(context.Background(), code, []store.ImportRecord{{Name: "x"}},
		map[string]string{"linux": "k0006959933"})
	if err != nil {
		t.Fatal(err)
	}
	if out[0].User != "k0006959933" {
		t.Fatalf("user = %q", out[0].User)
	}
}

func TestApplySandbox(t *testing.T) {
	code := `
function map(r)
  assert(os == nil, "os leaked")
  assert(io == nil, "io leaked")
  assert(dofile == nil, "dofile leaked")
  assert(loadfile == nil, "loadfile leaked")
  return r
end`
	out, err := Apply(context.Background(), code, []store.ImportRecord{{Name: "x"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatal("record should survive the sandbox assertions")
	}
}

func TestApplyTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := Apply(ctx, `function map(r) while true do end end`, []store.ImportRecord{{Name: "x"}}, nil)
	if err == nil {
		t.Fatal("infinite loop must be aborted by the context")
	}
}

func TestApplyErrors(t *testing.T) {
	if _, err := Apply(context.Background(), `function map(`, nil, nil); err == nil {
		t.Fatal("syntax error must fail")
	}
	if _, err := Apply(context.Background(), `x = 1`, nil, nil); err == nil || !strings.Contains(err.Error(), "must define map") {
		t.Fatalf("missing map(r): %v", err)
	}
	if _, err := Apply(context.Background(), `function map(r) return 42 end`, []store.ImportRecord{{Name: "x"}}, nil); err == nil {
		t.Fatal("non-table return must fail")
	}
}
