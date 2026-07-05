// Package updater checks GitHub Releases for a newer f9 build. It reads only the
// public releases API; it never downloads or replaces anything itself.
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const releasesAPI = "https://api.github.com/repos/%s/releases/latest"

// Info is the result of a check.
type Info struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
	Newer   bool   `json:"newer"`
	URL     string `json:"url"`
	Notes   string `json:"notes"`
	Error   string `json:"error"`
}

// Check queries the latest release of repo ("owner/name") and compares it to
// current. Errors are returned in Info.Error; it never panics.
func Check(ctx context.Context, repo, current string) Info {
	return check(ctx, fmt.Sprintf(releasesAPI, repo), current)
}

func check(ctx context.Context, url, current string) Info {
	info := Info{Current: current}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "f9-updater")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusNotFound {
		info.Error = "no releases published yet"
		return info
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		info.Error = fmt.Sprintf("github api %d", resp.StatusCode)
		return info
	}
	var rel struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Body    string `json:"body"`
	}
	if err := json.Unmarshal(body, &rel); err != nil {
		info.Error = err.Error()
		return info
	}
	info.Latest = strings.TrimPrefix(strings.TrimSpace(rel.TagName), "v")
	info.URL = rel.HTMLURL
	info.Notes = rel.Body
	info.Newer = isNewer(current, info.Latest)
	return info
}

// isNewer reports whether latest > current. A non-semver current (e.g. "dev")
// is never treated as older, so un-stamped builds never nag.
func isNewer(current, latest string) bool {
	c, okc := parse(current)
	l, okl := parse(latest)
	if !okc || !okl {
		return false
	}
	for i := 0; i < 3; i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i := 0; i < 3; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}
