package store

import "testing"

func TestOverlayKeyFileAndAgent(t *testing.T) {
	kf := "/home/u/.ssh/work"
	yes := true
	got := overlay(SessionOptions{}, SessionOptions{KeyFile: &kf, UseAgent: &yes})
	if got.KeyFile == nil || *got.KeyFile != kf {
		t.Fatalf("KeyFile not applied: %+v", got.KeyFile)
	}
	if got.UseAgent == nil || !*got.UseAgent {
		t.Fatalf("UseAgent not applied: %+v", got.UseAgent)
	}
	// a nil override inherits the base
	got2 := overlay(SessionOptions{KeyFile: &kf}, SessionOptions{})
	if got2.KeyFile == nil || *got2.KeyFile != kf {
		t.Fatal("KeyFile should inherit when the override is nil")
	}
}
