// Package xfer is the SFTP file-transfer engine behind the upload dialog:
// remote directory browsing and bounded, cancellable uploads over an existing
// SSH client. It knows nothing about sessions or the UI.
package xfer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Entry is one remote directory entry.
type Entry struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	Dir     bool      `json:"dir"`
	Mode    string    `json:"mode"`
	ModTime time.Time `json:"modTime"`
}

// Conn is an open SFTP channel. Not safe for concurrent directory ops and
// uploads on the same Conn from multiple goroutines beyond what pkg/sftp
// allows (it is request-pipelined and goroutine-safe).
type Conn struct {
	c *sftp.Client
}

// Open starts the sftp subsystem on an established SSH client. A server
// without SFTP (many network devices) fails here with a clear error.
func Open(client *ssh.Client) (*Conn, error) {
	if client == nil {
		return nil, errors.New("xfer: no SSH client")
	}
	c, err := sftp.NewClient(client, sftp.UseConcurrentWrites(true), sftp.MaxConcurrentRequestsPerFile(16))
	if err != nil {
		return nil, fmt.Errorf("xfer: sftp subsystem: %w (the server may not offer SFTP; scp-only devices are not supported yet)", err)
	}
	return &Conn{c: c}, nil
}

// Home returns the remote working directory (normally the login home).
func (x *Conn) Home() (string, error) {
	d, err := x.c.Getwd()
	if err != nil {
		return "", fmt.Errorf("xfer: getwd: %w", err)
	}
	return d, nil
}

// Clean normalizes a remote path: absolute, no trailing slash, "." segments
// collapsed. Relative input is resolved against the remote cwd.
func (x *Conn) Clean(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" || p == "~" {
		return x.Home()
	}
	if strings.HasPrefix(p, "~/") {
		h, err := x.Home()
		if err != nil {
			return "", err
		}
		p = h + "/" + p[2:]
	}
	if !path.IsAbs(p) {
		h, err := x.Home()
		if err != nil {
			return "", err
		}
		p = h + "/" + p
	}
	return path.Clean(p), nil
}

// List reads a directory: directories first, then files, both name-sorted.
func (x *Conn) List(dir string) ([]Entry, error) {
	infos, err := x.c.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("xfer: list %s: %w", dir, err)
	}
	out := make([]Entry, 0, len(infos))
	for _, fi := range infos {
		out = append(out, Entry{
			Name: fi.Name(), Size: fi.Size(), Dir: fi.IsDir(),
			Mode: fi.Mode().String(), ModTime: fi.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Mkdir creates one directory (parents must exist).
func (x *Conn) Mkdir(dir string) error {
	if err := x.c.Mkdir(dir); err != nil {
		return fmt.Errorf("xfer: mkdir %s: %w", dir, err)
	}
	return nil
}

// Upload copies a local file to remoteDir/<basename>. progress is called with
// cumulative bytes written (at most every 256 KiB and at the end); ctx cancels
// mid-transfer and removes the partial remote file.
func (x *Conn) Upload(ctx context.Context, local, remoteDir string, progress func(done, total int64)) error {
	lf, err := os.Open(local)
	if err != nil {
		return fmt.Errorf("xfer: open local: %w", err)
	}
	defer lf.Close()
	st, err := lf.Stat()
	if err != nil {
		return fmt.Errorf("xfer: stat local: %w", err)
	}
	if st.IsDir() {
		return fmt.Errorf("xfer: %s is a directory (upload files only)", local)
	}
	total := st.Size()
	remote := path.Join(remoteDir, path.Base(strings.ReplaceAll(local, "\\", "/")))
	rf, err := x.c.OpenFile(remote, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("xfer: create remote %s: %w", remote, err)
	}
	cw := &countingWriter{w: rf, total: total, progress: progress}
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(cw, lf)
		done <- err
	}()
	select {
	case err = <-done:
	case <-ctx.Done():
		_ = rf.Close()
		_ = lf.Close() // unblocks the copy goroutine
		<-done
		_ = x.c.Remove(remote)
		return ctx.Err()
	}
	if cerr := rf.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = x.c.Remove(remote)
		return fmt.Errorf("xfer: upload %s: %w", remote, err)
	}
	if progress != nil {
		progress(total, total)
	}
	return nil
}

type countingWriter struct {
	w        io.Writer
	n, total int64
	last     int64
	progress func(done, total int64)
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	if c.progress != nil && c.n-c.last >= 256<<10 {
		c.last = c.n
		c.progress(c.n, c.total)
	}
	return n, err
}

// Close ends the SFTP channel (not the SSH connection).
func (x *Conn) Close() error { return x.c.Close() }
