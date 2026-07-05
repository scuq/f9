package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFolderSourceFlowNoAuth(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"sessions":[{"id":"1","name":"sw1","host":"10.0.0.1"},{"id":"2","name":"sw2","host":"10.0.0.2"}]}`))
	}))
	defer srv.Close()

	a, st, labID := newTestApp(t)
	dto := SourceDTO{URL: srv.URL, Format: "f9-native", Auth: "none", ReconcileBy: "externalId", Insecure: true}
	if err := a.FolderSourceSet(labID, dto, ""); err != nil {
		t.Fatal(err)
	}
	if tr := a.FolderSourceTest(labID, dto, ""); !tr.OK || tr.Count != 2 {
		t.Fatalf("test = %+v", tr)
	}
	if rr := a.FolderSourceRefresh(labID); rr.Error != "" || rr.Added != 2 {
		t.Fatalf("refresh = %+v", rr)
	}
	n := 0
	for _, s := range st.Sessions() {
		if s.Source == labID {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("materialized %d generated sessions, want 2", n)
	}
}

func TestFolderSourceBearerAndLock(t *testing.T) {
	var gotAuth string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"sessions":[{"id":"1","name":"a","host":"10.0.0.1"}]}`))
	}))
	defer srv.Close()

	a, _, labID := newTestApp(t)
	if err := a.CredSetPassphrase("pw"); err != nil {
		t.Fatal(err)
	}
	dto := SourceDTO{URL: srv.URL, Format: "f9-native", Auth: "bearer", ReconcileBy: "hostname", Insecure: true}
	if err := a.FolderSourceSet(labID, dto, "tok999"); err != nil {
		t.Fatal(err)
	}
	if rr := a.FolderSourceRefresh(labID); rr.Error != "" {
		t.Fatalf("refresh: %s", rr.Error)
	}
	if gotAuth != "Bearer tok999" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	a.creds.Lock()
	if rr := a.FolderSourceRefresh(labID); rr.Error == "" {
		t.Fatal("refresh with locked cred store should error")
	}
}

func TestFolderSourceSetRequiresSecret(t *testing.T) {
	a, _, labID := newTestApp(t)
	if err := a.CredSetPassphrase("pw"); err != nil {
		t.Fatal(err)
	}
	dto := SourceDTO{URL: "https://x.example/api/", Format: "netbox", Auth: "bearer", ReconcileBy: "hostname"}
	if err := a.FolderSourceSet(labID, dto, ""); err == nil {
		t.Fatal("bearer without a secret should be rejected")
	}
}
