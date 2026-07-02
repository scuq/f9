package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/klauspost/compress/zstd"
)

// recordingsDir returns where session recordings live.
func recordingsDir() (string, error) {
	if p := os.Getenv("F9_RECORDINGS"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(home, ".config", "f9", "recordings"), nil
}

// recorder tees raw session output into a zstd-compressed file. Write is
// mutex-guarded because the stdout and stderr pumps may call the OnData
// handler concurrently.
type recorder struct {
	mu sync.Mutex
	f  *os.File
	zw *zstd.Encoder
	n  int64
}

func newRecorder(path string) (*recorder, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create recordings dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create recording: %w", err)
	}
	zw, err := zstd.NewWriter(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("zstd writer: %w", err)
	}
	return &recorder{f: f, zw: zw}, nil
}

func (r *recorder) Write(p []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.zw.Write(p); err != nil {
		return err
	}
	r.n += int64(len(p))
	return nil
}

// Bytes returns the uncompressed byte count written so far.
func (r *recorder) Bytes() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.n
}

func (r *recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.zw.Close(); err != nil {
		r.f.Close()
		return err
	}
	return r.f.Close()
}
