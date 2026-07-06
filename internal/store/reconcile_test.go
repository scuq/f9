package store

import "testing"

func TestReconcileFolderSessions(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := mkFolder(t, s, "lab", s.RootID())

	// a hand-made session that reconcile must never touch
	if err := s.Put(Session{Name: "manual", FolderID: f.ID, Host: "1.2.3.4"}); err != nil {
		t.Fatal(err)
	}

	recs := []ImportRecord{
		{ExternalID: "1", Name: "sw1", Host: "10.0.0.1"},
		{ExternalID: "2", Name: "sw2", Host: "10.0.0.2"},
	}
	res, err := s.ReconcileFolderSessions(f.ID, recs, "externalId")
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 2 || res.Updated != 0 || res.Removed != 0 {
		t.Fatalf("first reconcile = %+v", res)
	}

	// sw1 host changes, sw2 vanishes, sw3 appears
	recs2 := []ImportRecord{
		{ExternalID: "1", Name: "sw1", Host: "10.0.0.99"},
		{ExternalID: "3", Name: "sw3", Host: "10.0.0.3"},
	}
	res2, err := s.ReconcileFolderSessions(f.ID, recs2, "externalId")
	if err != nil {
		t.Fatal(err)
	}
	if res2.Added != 1 || res2.Updated != 1 || res2.Removed != 1 {
		t.Fatalf("second reconcile = %+v", res2)
	}

	var sawUpdated, sawManual bool
	for _, sess := range s.Sessions() {
		if sess.ExternalID == "1" {
			if sess.Host != "10.0.0.99" {
				t.Fatalf("sw1 host = %q, want updated", sess.Host)
			}
			sawUpdated = true
		}
		if sess.Name == "manual" && sess.Source == "" {
			sawManual = true
		}
	}
	if !sawUpdated {
		t.Fatal("sw1 not found after update")
	}
	if !sawManual {
		t.Fatal("hand-made session was clobbered")
	}
}

func TestReconcileByHostname(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := mkFolder(t, s, "lab", s.RootID())
	recs := []ImportRecord{{Name: "a", Host: "10.0.0.1"}, {Name: "b", Host: "10.0.0.2"}}
	if _, err := s.ReconcileFolderSessions(f.ID, recs, "hostname"); err != nil {
		t.Fatal(err)
	}
	// same hosts, new names -> update in place (2 updated, 0 added)
	recs2 := []ImportRecord{{Name: "a2", Host: "10.0.0.1"}, {Name: "b2", Host: "10.0.0.2"}}
	res, err := s.ReconcileFolderSessions(f.ID, recs2, "hostname")
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 2 || res.Added != 0 || res.Removed != 0 {
		t.Fatalf("hostname reconcile = %+v", res)
	}
}

func TestReconcileSkipsDuplicateNames(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := mkFolder(t, s, "nb", s.RootID())
	// Two devices with the same name but distinct external IDs (NetBox allows
	// per-site duplicate names). One imports; the other is skipped, not fatal.
	recs := []ImportRecord{
		{ExternalID: "1", Name: "dup", Host: "10.0.0.1"},
		{ExternalID: "2", Name: "dup", Host: "10.0.0.2"},
	}
	res, err := s.ReconcileFolderSessions(f.ID, recs, "externalId")
	if err != nil {
		t.Fatalf("reconcile must not abort on a duplicate name: %v", err)
	}
	if res.Added != 1 || res.Skipped != 1 {
		t.Fatalf("added=%d skipped=%d (want 1/1)", res.Added, res.Skipped)
	}
}
