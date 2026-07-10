package sessionimport

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scuq/f9/internal/store"
)

func TestFetchAllNetBoxPaginates(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("offset") == "" {
			fmt.Fprintf(w, `{"next":%q,"results":[{"id":1,"name":"a","primary_ip":{"address":"10.0.0.1/24"}}]}`, srv.URL+"/?offset=1")
			return
		}
		fmt.Fprint(w, `{"next":null,"results":[{"id":2,"name":"b","primary_ip":{"address":"10.0.0.2/24"}}]}`)
	}))
	defer srv.Close()

	src := store.FolderSource{URL: srv.URL, Format: "netbox", Insecure: true}
	recs, err := FetchAll(context.Background(), src, "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || recs[0].Host != "10.0.0.1" || recs[1].Host != "10.0.0.2" {
		t.Fatalf("recs = %+v", recs)
	}
}

func TestFetchAllNetBoxCrossOriginRefused(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"next":"https://evil.example/api/","results":[{"id":1,"name":"a"}]}`)
	}))
	defer srv.Close()
	src := store.FolderSource{URL: srv.URL, Format: "netbox", Insecure: true}
	if _, err := FetchAll(context.Background(), src, "", nil, false); err == nil {
		t.Fatal("expected cross-origin pagination to be refused")
	}
}
