package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		cur, lat string
		want     bool
	}{
		{"1.0.0", "1.0.1", true},
		{"1.0.0", "1.1.0", true},
		{"1.0.0", "2.0.0", true},
		{"1.2.3", "1.2.3", false},
		{"1.2.3", "1.2.2", false},
		{"2.0.0", "1.9.9", false},
		{"dev", "1.0.0", false},
		{"1.0.0", "v1.0.1", true},
		{"1.0.0-rc1", "1.0.0", false},
	}
	for _, c := range cases {
		if got := isNewer(c.cur, c.lat); got != c.want {
			t.Errorf("isNewer(%q,%q)=%v want %v", c.cur, c.lat, got, c.want)
		}
	}
}

func TestCheckParsesAndCompares(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.0","html_url":"https://x/rel","body":"changelog"}`))
	}))
	defer srv.Close()

	info := check(context.Background(), srv.URL, "1.1.0")
	if info.Error != "" {
		t.Fatal(info.Error)
	}
	if info.Latest != "1.2.0" || !info.Newer || info.URL != "https://x/rel" {
		t.Fatalf("info = %+v", info)
	}
	if check(context.Background(), srv.URL, "1.2.0").Newer {
		t.Fatal("same version should not be newer")
	}
}

func TestCheck404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()
	if check(context.Background(), srv.URL, "1.0.0").Error == "" {
		t.Fatal("expected error on 404")
	}
}
