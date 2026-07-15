package store

import "testing"

func TestOSHintRoundtripAndPinned(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.PutOSHint(OSHint{Host: "10.0.0.1", OS: "ios", Confidence: 0.9}); err != nil {
		t.Fatal(err)
	}
	h, ok := s.OSHint("10.0.0.1")
	if !ok || h.OS != "ios" {
		t.Fatalf("hint = %+v ok=%v", h, ok)
	}
	if _, ok := s.OSHint("  10.0.0.1 "); !ok {
		t.Fatal("normalized lookup failed")
	}
	// a pinned hint is only replaced by another pinned write
	if err := s.PutOSHint(OSHint{Host: "10.0.0.1", OS: "nxos", Pinned: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutOSHint(OSHint{Host: "10.0.0.1", OS: "linux"}); err != nil {
		t.Fatal(err)
	}
	h, _ = s.OSHint("10.0.0.1")
	if h.OS != "nxos" || !h.Pinned {
		t.Fatalf("pinned hint overwritten: %+v", h)
	}
	// delete removes even a pinned hint (explicit user action)
	if err := s.DeleteOSHint("10.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.OSHint("10.0.0.1"); ok {
		t.Fatal("hint still present after delete")
	}
	if err := s.DeleteOSHint("10.0.0.1"); err != nil {
		t.Fatal("delete of missing hint must be a no-op, got error")
	}
}

func TestReconcileJoinsOSHint(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := mkFolder(t, s, "lab", s.RootID())
	if err := s.PutOSHint(OSHint{Host: "10.0.0.7", OS: "ios", Confidence: 0.9}); err != nil {
		t.Fatal(err)
	}
	recs := []ImportRecord{{ExternalID: "7", Name: "sw7", Host: "10.0.0.7"}}
	if _, err := s.ReconcileFolderSessions(f.ID, recs, "hostname"); err != nil {
		t.Fatal(err)
	}
	created, ok := s.SessionByName(f.ID, "sw7")
	if !ok {
		t.Fatal("sw7 missing")
	}
	if m, err := s.Meta(created.ID); err != nil || m.DetectedOS != "ios" {
		t.Fatalf("meta = %+v err=%v, want ios", m, err)
	}
	// remove and re-import: a brand-new session ID still knows its OS
	if _, err := s.ReconcileFolderSessions(f.ID, []ImportRecord{}, "hostname"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReconcileFolderSessions(f.ID, recs, "hostname"); err != nil {
		t.Fatal(err)
	}
	again, ok := s.SessionByName(f.ID, "sw7")
	if !ok {
		t.Fatal("sw7 missing after re-add")
	}
	if again.ID == created.ID {
		t.Fatal("expected a recreated session (new ID)")
	}
	if m, err := s.Meta(again.ID); err != nil || m.DetectedOS != "ios" {
		t.Fatalf("re-add meta = %+v err=%v, want ios (hint join)", m, err)
	}
}
