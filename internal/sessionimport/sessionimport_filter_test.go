package sessionimport

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scuq/f9/internal/store"
)

func TestFetchAllNetBoxFilters(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"next":null,"results":[
			{"id":1,"name":"core-1","role":{"name":"core"}},
			{"id":2,"name":"edge-1","role":{"name":"edge"}}
		]}`)
	}))
	defer srv.Close()

	src := store.FolderSource{
		URL: srv.URL, Format: "netbox", Insecure: true,
		Filter: &store.FilterGroup{Rules: []store.FilterRule{{Field: "role", Kind: "eq", Value: "core"}}},
	}
	recs, err := FetchAll(context.Background(), src, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Name != "core-1" {
		t.Fatalf("recs = %+v", recs)
	}
}
