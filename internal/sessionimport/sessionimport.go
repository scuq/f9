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
	"net/url"
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
	recs, _, err := decodeNetBoxPage(body)
	return recs, err
}

// decodeNetBoxPage decodes one NetBox page and returns the records plus the URL
// of the next page ("" when there are no more).
func decodeNetBoxPage(body []byte) ([]store.ImportRecord, string, error) {
	var doc struct {
		Next    string `json:"next"`
		Results []struct {
			ID     int    `json:"id"`
			Name   string `json:"name"`
			Status *struct {
				Value string `json:"value"`
			} `json:"status"`
			Role *struct {
				Name string `json:"name"`
			} `json:"role"`
			DeviceRole *struct {
				Name string `json:"name"`
			} `json:"device_role"`
			DeviceType *struct {
				Model        string `json:"model"`
				Manufacturer *struct {
					Name string `json:"name"`
				} `json:"manufacturer"`
			} `json:"device_type"`
			Tenant *struct {
				Name string `json:"name"`
			} `json:"tenant"`
			Site *struct {
				Name string `json:"name"`
			} `json:"site"`
			PrimaryIP *struct {
				Address string `json:"address"`
			} `json:"primary_ip"`
			CustomFields map[string]interface{} `json:"custom_fields"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, "", fmt.Errorf("sessionimport: netbox decode: %w", err)
	}
	// second, generic pass over the same body: the raw object per device, for
	// the Lua map hook (r.raw).
	var rawDoc struct {
		Results []map[string]interface{} `json:"results"`
	}
	_ = json.Unmarshal(body, &rawDoc)
	out := make([]store.ImportRecord, 0, len(doc.Results))
	for i, d := range doc.Results {
		host := d.Name
		if d.PrimaryIP != nil && d.PrimaryIP.Address != "" {
			host = d.PrimaryIP.Address
			if i := strings.IndexByte(host, '/'); i >= 0 {
				host = host[:i]
			}
		}
		attrs := map[string]string{"hostname": d.Name}
		if d.Status != nil {
			attrs["status"] = d.Status.Value
		}
		if d.Role != nil {
			attrs["role"] = d.Role.Name
		} else if d.DeviceRole != nil {
			attrs["role"] = d.DeviceRole.Name
		}
		if d.DeviceType != nil {
			attrs["model"] = d.DeviceType.Model
			if d.DeviceType.Manufacturer != nil {
				attrs["manufacturer"] = d.DeviceType.Manufacturer.Name
			}
		}
		if d.Tenant != nil {
			attrs["tenant"] = d.Tenant.Name
		}
		if d.Site != nil {
			attrs["site"] = d.Site.Name
		}
		for k, cv := range d.CustomFields {
			switch val := cv.(type) {
			case string:
				attrs["cf:"+k] = val
			case bool:
				attrs["cf:"+k] = strconv.FormatBool(val)
			case float64:
				attrs["cf:"+k] = strconv.FormatFloat(val, 'f', -1, 64)
			case map[string]interface{}:
				if sv, ok := val["value"].(string); ok {
					attrs["cf:"+k] = sv
				} else if lv, ok := val["label"].(string); ok {
					attrs["cf:"+k] = lv
				}
			}
		}
		rec := store.ImportRecord{
			ExternalID: strconv.Itoa(d.ID), Name: d.Name, Host: host, Port: 22, Proto: "ssh", Attrs: attrs,
		}
		if i < len(rawDoc.Results) {
			rec.Raw = rawDoc.Results[i]
		}
		out = append(out, rec)
	}
	return out, doc.Next, nil
}

// maxPages bounds pagination so a misbehaving endpoint can't loop forever.
const maxPages = 500

// netboxPageSize is the requested page size. NetBox caps it at its own
// MAX_PAGE_SIZE (default 1000); a large value keeps the round-trip count low.
const netboxPageSize = 1000

// Preview (test) bounds: keep fetching pages until this many filter matches
// are found, up to previewMaxPages — so late-sorting matches still show up
// without scanning the whole source.
const previewTargetMatches = 25
const previewMaxPages = 8

// FetchAll fetches all records for a source. The netbox format follows the
// paginated `next` links (same-origin only); other formats are a single fetch.
// fieldMap is used only by the mapped format.
// FetchStats reports how much of the source a fetch covered.
type FetchStats struct {
	Scanned int  // records decoded
	Partial bool // true when a preview stopped before the end of the source
}

// FetchAll fetches records for a source. When preview is true it fetches pages
// only until it has previewTargetMatches filter matches (or previewMaxPages),
// reporting Partial when it stopped early; refresh passes false to page
// through everything.
func FetchAll(ctx context.Context, src store.FolderSource, secret string, fieldMap map[string]string, preview bool) ([]store.ImportRecord, FetchStats, error) {
	if src.Format != "netbox" {
		body, err := Fetch(ctx, src, secret)
		if err != nil {
			return nil, FetchStats{}, err
		}
		recs, err := Decode(src.Format, fieldMap, body)
		if err != nil {
			return nil, FetchStats{}, err
		}
		return recs, FetchStats{Scanned: len(recs)}, nil
	}
	base, err := url.Parse(src.URL)
	if err != nil {
		return nil, FetchStats{}, fmt.Errorf("sessionimport: url: %w", err)
	}
	// Request a large page size to minimize round-trips; NetBox caps it at its
	// MAX_PAGE_SIZE and the `next` links carry the effective limit forward.
	if q := base.Query(); q.Get("limit") == "" {
		q.Set("limit", strconv.Itoa(netboxPageSize))
		base.RawQuery = q.Encode()
	}
	match, err := store.CompileFilter(src.Filter)
	if err != nil {
		return nil, FetchStats{}, err
	}
	var all []store.ImportRecord
	next := base.String()
	for page := 0; next != "" && page < maxPages; page++ {
		nu, err := url.Parse(next)
		if err != nil {
			return nil, FetchStats{}, fmt.Errorf("sessionimport: next url: %w", err)
		}
		if nu.Scheme != base.Scheme || nu.Host != base.Host {
			return nil, FetchStats{}, fmt.Errorf("sessionimport: refusing cross-origin pagination to %q", nu.Host)
		}
		pageSrc := src
		pageSrc.URL = next
		body, err := Fetch(ctx, pageSrc, secret)
		if err != nil {
			return nil, FetchStats{}, err
		}
		recs, nx, err := decodeNetBoxPage(body)
		if err != nil {
			return nil, FetchStats{}, err
		}
		all = append(all, recs...)
		next = nx
		if preview {
			matched := 0
			for _, r := range all {
				if match(r.Attrs) {
					matched++
				}
			}
			if matched >= previewTargetMatches || page+1 >= previewMaxPages {
				break
			}
		}
	}
	stats := FetchStats{Scanned: len(all), Partial: next != ""}
	filtered := all[:0]
	for _, r := range all {
		if match(r.Attrs) {
			filtered = append(filtered, r)
		}
	}
	return filtered, stats, nil
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
