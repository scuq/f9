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
	recs, err := FetchAll(context.Background(), src, "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Name != "core-1" {
		t.Fatalf("recs = %+v", recs)
	}
}

func TestFetchAllNetBoxCustomFieldFilter(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"next":null,"results":[
			{"id":1,"name":"a","custom_fields":{"cmdbSupportTeam":"IT-EDVKP/3400"}},
			{"id":2,"name":"b","custom_fields":{"cmdbSupportTeam":"IT-OTHER/2000"}}
		]}`)
	}))
	defer srv.Close()

	src := store.FolderSource{
		URL: srv.URL, Format: "netbox", Insecure: true,
		Filter: &store.FilterGroup{Rules: []store.FilterRule{{Field: "cf:cmdbSupportTeam", Kind: "contains", Value: "3400"}}},
	}
	recs, err := FetchAll(context.Background(), src, "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Name != "a" {
		t.Fatalf("recs = %+v", recs)
	}
}
