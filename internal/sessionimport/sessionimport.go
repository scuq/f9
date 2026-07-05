// Package sessionimport fetches and decodes sessions from an external HTTPS
// source (per-folder). It performs no storage; the store reconciles the
// returned records. Auth material comes from the cred store (never SSH auth).
package sessionimport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/scuq/f9/internal/store"
)

const bodyCap = 16 << 20 // 16 MiB

// Fetch retrieves the raw response body from a source endpoint, applying auth.
// secret carries credential material appropriate to src.Auth:
//
//	bearer -> token (Authorization: Bearer <secret>), or, if src.Header is set,
//	          that header is set verbatim to <secret> (e.g. "Token abc123")
//	basic  -> "user:password"
//	mtls   -> a client cert+key PEM bundle
func Fetch(ctx context.Context, src store.FolderSource, secret string) ([]byte, error) {
	return fetch(ctx, src, secret, nil)
}

func fetch(ctx context.Context, src store.FolderSource, secret string, roots *x509.CertPool) ([]byte, error) {
	// InsecureSkipVerify is an explicit per-source opt-in for untrusted remote
	// certificates; it is never the default.
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, InsecureSkipVerify: src.Insecure} //nolint:gosec
	if src.Auth == "mtls" {
		cert, err := tls.X509KeyPair([]byte(secret), []byte(secret))
		if err != nil {
			return nil, fmt.Errorf("sessionimport: mtls keypair: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("sessionimport: request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	switch src.Auth {
	case "bearer":
		if src.Header != "" {
			req.Header.Set(src.Header, secret)
		} else {
			req.Header.Set("Authorization", "Bearer "+secret)
		}
	case "basic":
		user, pass, _ := strings.Cut(secret, ":")
		req.SetBasicAuth(user, pass)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sessionimport: get: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, bodyCap))
	if err != nil {
		return nil, fmt.Errorf("sessionimport: read: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(body)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, fmt.Errorf("sessionimport: http %d: %s", resp.StatusCode, strings.TrimSpace(snippet))
	}
	return body, nil
}

// Decode turns a response body into import records for the given format.
func Decode(format string, fieldMap map[string]string, body []byte) ([]store.ImportRecord, error) {
	switch format {
	case "f9-native":
		return decodeNative(body)
	case "netbox":
		return decodeNetBox(body)
	case "mapped":
		return decodeMapped(fieldMap, body)
	default:
		return nil, fmt.Errorf("sessionimport: unknown format %q", format)
	}
}

func decodeNative(body []byte) ([]store.ImportRecord, error) {
	var doc struct {
		Sessions []struct {
			ID    string   `json:"id"`
			Name  string   `json:"name"`
			Host  string   `json:"host"`
			Port  int      `json:"port"`
			User  string   `json:"user"`
			Proto string   `json:"proto"`
			Tags  []string `json:"tags"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("sessionimport: native decode: %w", err)
	}
	out := make([]store.ImportRecord, 0, len(doc.Sessions))
	for _, x := range doc.Sessions {
		out = append(out, store.ImportRecord{
			ExternalID: x.ID, Name: x.Name, Host: x.Host, Port: x.Port,
			User: x.User, Proto: x.Proto, Tags: x.Tags,
		})
	}
	return out, nil
}

func decodeNetBox(body []byte) ([]store.ImportRecord, error) {
	var doc struct {
		Results []struct {
			ID        int    `json:"id"`
			Name      string `json:"name"`
			PrimaryIP *struct {
				Address string `json:"address"`
			} `json:"primary_ip"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("sessionimport: netbox decode: %w", err)
	}
	out := make([]store.ImportRecord, 0, len(doc.Results))
	for _, d := range doc.Results {
		host := d.Name
		if d.PrimaryIP != nil && d.PrimaryIP.Address != "" {
			host = d.PrimaryIP.Address
			if i := strings.IndexByte(host, '/'); i >= 0 {
				host = host[:i]
			}
		}
		out = append(out, store.ImportRecord{
			ExternalID: strconv.Itoa(d.ID), Name: d.Name, Host: host, Port: 22, Proto: "ssh",
		})
	}
	return out, nil
}

func decodeMapped(fieldMap map[string]string, body []byte) ([]store.ImportRecord, error) {
	if len(fieldMap) == 0 {
		return nil, fmt.Errorf("sessionimport: mapped format requires a field_map")
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("sessionimport: mapped decode (expect a JSON array): %w", err)
	}
	get := func(row map[string]any, f9field string) string {
		key, ok := fieldMap[f9field]
		if !ok {
			return ""
		}
		v, ok := row[key]
		if !ok || v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
	out := make([]store.ImportRecord, 0, len(rows))
	for _, row := range rows {
		port := 22
		if ps := get(row, "port"); ps != "" {
			if p, err := strconv.Atoi(ps); err == nil {
				port = p
			}
		}
		proto := get(row, "proto")
		if proto == "" {
			proto = "ssh"
		}
		out = append(out, store.ImportRecord{
			ExternalID: get(row, "externalId"),
			Name:       get(row, "name"),
			Host:       get(row, "host"),
			Port:       port,
			User:       get(row, "user"),
			Proto:      proto,
		})
	}
	return out, nil
}
