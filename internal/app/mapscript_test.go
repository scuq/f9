package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFolderSourceTestWithMapScript(t *testing.T) {
	a, _, folderID := newTestApp(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"next":null,"results":[{"id":1,"name":"a"},{"id":2,"name":"b"}]}`)
	}))
	defer srv.Close()

	if err := a.MapScriptPut("ren", `function map(r)
  if r.name == "b" then return nil end
  r.name = r.name .. "_x"
  return r
end`); err != nil {
		t.Fatal(err)
	}

	dto := SourceDTO{URL: srv.URL, Format: "netbox", Auth: "none", ReconcileBy: "externalId", Insecure: true, MapScript: "ren"}
	tr := a.FolderSourceTest(folderID, dto, "")
	if tr.Error != "" {
		t.Fatal(tr.Error)
	}
	if tr.Count != 1 || len(tr.Sample) == 0 || tr.Sample[0] != "a_x" {
		t.Fatalf("test result = %+v", tr)
	}

	dto.MapScript = "missing"
	if tr := a.FolderSourceTest(folderID, dto, ""); tr.Error == "" {
		t.Fatal("missing script must error")
	}
}
