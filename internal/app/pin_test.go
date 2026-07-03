package app

import "testing"

func TestPinUnpin(t *testing.T) {
	a, _, labID := newTestApp(t)
	id, err := a.SaveSession(SessionInput{FolderID: labID, Name: "p", Host: "h"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.PinSession(id); err != nil {
		t.Fatal(err)
	}
	ps, err := a.PinnedSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].ID != id || !ps[0].Pinned {
		t.Fatalf("pinned = %+v", ps)
	}
	if err := a.UnpinSession(id); err != nil {
		t.Fatal(err)
	}
	ps, _ = a.PinnedSessions()
	if len(ps) != 0 {
		t.Fatalf("still pinned: %+v", ps)
	}
}
