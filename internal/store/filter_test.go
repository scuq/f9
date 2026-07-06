package store

import "testing"

func TestCompileFilter(t *testing.T) {
	dev := map[string]string{
		"status": "active", "role": "NE", "hostname": "NW0102-O71A",
		"manufacturer": "CI", "model": "CI_CA-C9300LM-48UX-4Y",
	}

	m, err := CompileFilter(nil)
	if err != nil || !m(dev) {
		t.Fatal("nil filter should match all")
	}

	// AND, eq is case-insensitive
	m, err = CompileFilter(&FilterGroup{Op: "and", Rules: []FilterRule{
		{Field: "status", Kind: "eq", Value: "active"},
		{Field: "role", Kind: "eq", Value: "ne"},
	}})
	if err != nil || !m(dev) {
		t.Fatal("AND status+role should match")
	}

	// regex on hostname
	m, _ = CompileFilter(&FilterGroup{Rules: []FilterRule{{Field: "hostname", Kind: "regex", Value: "^NW0102-"}}})
	if !m(dev) {
		t.Fatal("hostname regex should match")
	}

	// negate
	m, _ = CompileFilter(&FilterGroup{Rules: []FilterRule{{Field: "status", Kind: "eq", Value: "active", Negate: true}}})
	if m(dev) {
		t.Fatal("negated status=active should not match")
	}

	// OR of groups
	m, _ = CompileFilter(&FilterGroup{Op: "or", Groups: []FilterGroup{
		{Rules: []FilterRule{{Field: "role", Kind: "eq", Value: "core"}}},
		{Rules: []FilterRule{{Field: "manufacturer", Kind: "eq", Value: "CI"}}},
	}})
	if !m(dev) {
		t.Fatal("OR group (manufacturer=CI) should match")
	}

	// invalid regex -> compile error
	if _, err := CompileFilter(&FilterGroup{Rules: []FilterRule{{Field: "hostname", Kind: "regex", Value: "("}}}); err == nil {
		t.Fatal("invalid regex should error at compile")
	}
}
