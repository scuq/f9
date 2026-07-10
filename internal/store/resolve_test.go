package store

import (
	"reflect"
	"testing"
	"time"
)

func strp(v string) *string               { return &v }
func intp(v int) *int                     { return &v }
func durp(v time.Duration) *time.Duration { return &v }

func TestOverlayFieldCoverage(t *testing.T) {
	// Guard: when SessionOptions grows, overlay() and the inheritance test
	// below must be extended. Bump the expected count last.
	if n := reflect.TypeOf(SessionOptions{}).NumField(); n != 10 {
		t.Fatalf("SessionOptions has %d fields; update overlay() and this test", n)
	}
}

func TestOverlayInheritance(t *testing.T) {
	root := SessionOptions{TermType: strp("xterm-256color"), KeepaliveInterval: durp(30 * time.Second)}
	folder := SessionOptions{TermType: strp("vt100"), JumpChain: []JumpHop{{Host: "jump1", Mode: "proxyjump"}}}
	session := SessionOptions{ScrollbackLines: intp(1000000)}

	eff := overlay(overlay(overlay(SessionOptions{}, root), folder), session)

	if eff.TermType == nil || *eff.TermType != "vt100" {
		t.Fatalf("TermType: want folder override vt100, got %v", eff.TermType)
	}
	if eff.KeepaliveInterval == nil || *eff.KeepaliveInterval != 30*time.Second {
		t.Fatalf("KeepaliveInterval: want inherited 30s, got %v", eff.KeepaliveInterval)
	}
	if len(eff.JumpChain) != 1 || eff.JumpChain[0].Host != "jump1" {
		t.Fatalf("JumpChain: want inherited jump1, got %v", eff.JumpChain)
	}
	if eff.ScrollbackLines == nil || *eff.ScrollbackLines != 1000000 {
		t.Fatalf("ScrollbackLines: want session 1000000, got %v", eff.ScrollbackLines)
	}
	if eff.Reconnect != nil || eff.ThemeRef != nil || eff.AuditScope != nil {
		t.Fatalf("unset fields must stay nil: %+v", eff)
	}
}
