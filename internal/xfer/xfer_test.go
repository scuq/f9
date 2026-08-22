package xfer

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// startSFTPServer runs an in-process SSH server whose only feature is the
// sftp subsystem (pkg/sftp's server over the real filesystem).
func startSFTPServer(t *testing.T) *ssh.Client {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				sconn, chans, reqs, err := ssh.NewServerConn(c, cfg)
				if err != nil {
					return
				}
				defer sconn.Close()
				go ssh.DiscardRequests(reqs)
				for nc := range chans {
					if nc.ChannelType() != "session" {
						nc.Reject(ssh.UnknownChannelType, "")
						continue
					}
					ch, creqs, err := nc.Accept()
					if err != nil {
						continue
					}
					go func() {
						for r := range creqs {
							ok := r.Type == "subsystem" && len(r.Payload) >= 4 && string(r.Payload[4:]) == "sftp"
							if r.WantReply {
								r.Reply(ok, nil)
							}
							if ok {
								srv, err := sftp.NewServer(ch)
								if err == nil {
									_ = srv.Serve()
								}
								ch.Close()
							}
						}
					}()
				}
			}()
		}
	}()
	client, err := ssh.Dial("tcp", ln.Addr().String(), &ssh.ClientConfig{
		User: "t", HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func TestListMkdirUpload(t *testing.T) {
	root := t.TempDir()
	x, err := Open(startSFTPServer(t))
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()

	if _, err := x.Home(); err != nil {
		t.Fatal(err)
	}
	if err := x.Mkdir(filepath.ToSlash(filepath.Join(root, "incoming"))); err != nil {
		t.Fatal(err)
	}
	ents, err := x.List(filepath.ToSlash(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 || ents[0].Name != "incoming" || !ents[0].Dir {
		t.Fatalf("list after mkdir: %+v", ents)
	}

	local := filepath.Join(root, "payload.bin")
	data := bytes.Repeat([]byte("f9-sftp-"), 100_000) // 800 KB: crosses the progress granularity
	if err := os.WriteFile(local, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var calls int
	var last int64
	err = x.Upload(context.Background(), local, filepath.ToSlash(filepath.Join(root, "incoming")), func(done, total int64) {
		calls++
		last = done
		if total != int64(len(data)) {
			t.Errorf("total = %d", total)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls < 2 || last != int64(len(data)) {
		t.Fatalf("progress: %d calls, last=%d", calls, last)
	}
	got, err := os.ReadFile(filepath.Join(root, "incoming", "payload.bin"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("remote content mismatch (err=%v, %d bytes)", err, len(got))
	}

	// cancelled upload removes the partial file
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = x.Upload(ctx, local, filepath.ToSlash(root), nil)
	if err == nil {
		t.Fatal("cancelled upload returned nil")
	}
	if _, serr := os.Stat(filepath.Join(root, "payload.bin")); serr == nil {
		// a fast machine may finish the copy before ctx is observed; tolerate
		// only when the file is complete
		if st, _ := os.Stat(filepath.Join(root, "payload.bin")); st.Size() != int64(len(data)) {
			t.Fatal("partial remote file left behind after cancel")
		}
	}
	// directory upload is refused
	if err := x.Upload(context.Background(), root, filepath.ToSlash(root), nil); err == nil {
		t.Fatal("directory upload must fail")
	}
}

func TestCleanPaths(t *testing.T) {
	x, err := Open(startSFTPServer(t))
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	home, _ := x.Home()
	for in, want := range map[string]string{"": home, "~": home, "/a/b/../c/": "/a/c", "/": "/"} {
		got, err := x.Clean(in)
		if err != nil || got != want {
			t.Fatalf("Clean(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}
