package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestRecorderRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.zst")
	r, err := newRecorder(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Write([]byte("show version\r\n")); err != nil {
		t.Fatal(err)
	}
	if err := r.Write([]byte("Cisco IOS Software\r\n")); err != nil {
		t.Fatal(err)
	}
	if got := r.Bytes(); got != 34 {
		t.Fatalf("Bytes = %d, want 34", got)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := zstd.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	data, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "show version\r\nCisco IOS Software\r\n" {
		t.Fatalf("roundtrip mismatch: %q", data)
	}
}
