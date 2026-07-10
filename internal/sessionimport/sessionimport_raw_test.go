package sessionimport

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scuq/f9/internal/store"
)

func TestFetchAllNetBoxCapturesRaw(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"next":null,"results":[{"id":1,"name":"a","preferedIpAddress":"10.9.9.9"}]}`)
	}))
	defer srv.Close()
	src := store.FolderSource{URL: srv.URL, Format: "netbox", Insecure: true}
	recs, err := FetchAll(context.Background(), src, "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Raw == nil || recs[0].Raw["preferedIpAddress"] != "10.9.9.9" {
		t.Fatalf("raw not captured: %+v", recs)
	}
}
