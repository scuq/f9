package sessionimport

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/scuq/f9/internal/store"
)

func tlsServer(t *testing.T, h http.HandlerFunc) (*httptest.Server, *x509.CertPool) {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	return srv, pool
}

func genCertPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "f9-test-client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return string(certPEM) + string(keyPEM)
}

func TestFetchBearerDefault(t *testing.T) {
	var got string
	srv, pool := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.Write([]byte(`{"sessions":[]}`))
	})
	src := store.FolderSource{URL: srv.URL, Auth: "bearer"}
	if _, err := fetch(context.Background(), src, "tok123", pool); err != nil {
		t.Fatal(err)
	}
	if got != "Bearer tok123" {
		t.Fatalf("auth header = %q", got)
	}
}

func TestFetchCustomHeader(t *testing.T) {
	var got string
	srv, pool := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.Write([]byte(`{"results":[]}`))
	})
	src := store.FolderSource{URL: srv.URL, Auth: "bearer", Header: "Authorization"}
	if _, err := fetch(context.Background(), src, "Token abc", pool); err != nil {
		t.Fatal(err)
	}
	if got != "Token abc" {
		t.Fatalf("auth header = %q", got)
	}
}

func TestFetchBasic(t *testing.T) {
	var user, pass string
	var ok bool
	srv, pool := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok = r.BasicAuth()
		w.Write([]byte(`[]`))
	})
	src := store.FolderSource{URL: srv.URL, Auth: "basic"}
	if _, err := fetch(context.Background(), src, "alice:s3cret", pool); err != nil {
		t.Fatal(err)
	}
	if !ok || user != "alice" || pass != "s3cret" {
		t.Fatalf("basic = %q/%q ok=%v", user, pass, ok)
	}
}

func TestFetchMTLS(t *testing.T) {
	var peers int
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil {
			peers = len(r.TLS.PeerCertificates)
		}
		w.Write([]byte(`{"sessions":[]}`))
	}))
	srv.TLS = &tls.Config{ClientAuth: tls.RequestClientCert}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())

	src := store.FolderSource{URL: srv.URL, Auth: "mtls"}
	if _, err := fetch(context.Background(), src, genCertPEM(t), pool); err != nil {
		t.Fatal(err)
	}
	if peers < 1 {
		t.Fatal("server saw no client certificate")
	}
}

func TestFetchHTTPError(t *testing.T) {
	srv, pool := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	})
	src := store.FolderSource{URL: srv.URL, Auth: "bearer"}
	if _, err := fetch(context.Background(), src, "x", pool); err == nil {
		t.Fatal("expected error on 401")
	}
}

func TestDecodeNative(t *testing.T) {
	body := []byte(`{"sessions":[{"id":"a1","name":"sw1","host":"10.0.0.1","port":2222,"user":"admin","proto":"ssh","tags":["core"]}]}`)
	recs, err := Decode("f9-native", nil, body)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].ExternalID != "a1" || recs[0].Host != "10.0.0.1" || recs[0].Port != 2222 {
		t.Fatalf("native = %+v", recs)
	}
}

func TestDecodeNetBox(t *testing.T) {
	body := []byte(`{"results":[{"id":7,"name":"sw2","primary_ip":{"address":"10.0.0.2/24"}}]}`)
	recs, err := Decode("netbox", nil, body)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].ExternalID != "7" || recs[0].Host != "10.0.0.2" || recs[0].Name != "sw2" {
		t.Fatalf("netbox = %+v", recs)
	}
}

func TestDecodeMapped(t *testing.T) {
	body := []byte(`[{"hostname":"sw3","mgmt":"10.0.0.3","uuid":"u3"}]`)
	fm := map[string]string{"name": "hostname", "host": "mgmt", "externalId": "uuid"}
	recs, err := Decode("mapped", fm, body)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Name != "sw3" || recs[0].Host != "10.0.0.3" || recs[0].ExternalID != "u3" || recs[0].Port != 22 {
		t.Fatalf("mapped = %+v", recs)
	}
	if _, err := Decode("mapped", nil, body); err == nil {
		t.Fatal("mapped without field_map should error")
	}
}
