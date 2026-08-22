// Package filter is the session fuzzy scorer behind the tree filter box.
// Contract (docs/phase-plan.md 01): ranking 10k sessions must stay under
// 5ms per query — enforced by TestFilterBudget. Dependency-free.
package filter

import (
	"regexp"
	"sort"
	"strings"
)

// Item is one filterable session.
type Item struct {
	ID   string
	Name string
	Path string // full folder path, e.g. Sessions/lab
	Host string
	Tags []string
}

// Hit is a ranked match.
type Hit struct {
	Item
	Score int
}

// Field weights: what you type is most likely a session name, then a host,
// then a tag, then a folder path segment.
const (
	weightName = 100
	weightHost = 60
	weightTag  = 50
	weightPath = 40
)

// Options tunes which fields Rank considers. Name, host and tags are always
// matched; the folder path only when MatchPath is set.
type Options struct {
	MatchPath bool
}

// Rank scores items against query over every field (including the folder
// path) and returns matches, best first (ties by name). Empty query returns
// all items in input order with score 0.
func Rank(query string, items []Item) []Hit {
	return RankWith(query, items, Options{MatchPath: true})
}

// RankWith is Rank with explicit field Options.
//
// Query forms: a plain query is a case-insensitive literal substring — every
// character must appear contiguously (no scattered-subsequence fuzzing, which
// made "0102" hit "NWR501-HB01-125A"). A query wrapped in slashes, /re/, is a
// case-insensitive Go regexp; an invalid pattern matches nothing.
func RankWith(query string, items []Item, opts Options) []Hit {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		out := make([]Hit, len(items))
		for i, it := range items {
			out[i] = Hit{Item: it}
		}
		return out
	}
	var re *regexp.Regexp
	if len(q) >= 2 && strings.HasPrefix(q, "/") && strings.HasSuffix(q, "/") {
		pat := strings.TrimSpace(query)
		pat = pat[1 : len(pat)-1]
		if pat == "" {
			return nil
		}
		var err error
		re, err = regexp.Compile("(?i)" + pat)
		if err != nil {
			return nil
		}
	}
	match := func(field string, weight int) int {
		if re != nil {
			return regexScore(re, field, weight)
		}
		return fieldScore(q, field, weight)
	}
	out := make([]Hit, 0, 32)
	for _, it := range items {
		s := match(it.Name, weightName)
		if v := match(it.Host, weightHost); v > s {
			s = v
		}
		if opts.MatchPath {
			if v := match(it.Path, weightPath); v > s {
				s = v
			}
		}
		for _, tag := range it.Tags {
			if v := match(tag, weightTag); v > s {
				s = v
			}
		}
		if s > 0 {
			out = append(out, Hit{Item: it, Score: s})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// regexScore scores a /re/ query: leftmost match position and shorter
// targets rank higher, mirroring the substring bonuses.
func regexScore(re *regexp.Regexp, s string, weight int) int {
	if s == "" {
		return 0
	}
	loc := re.FindStringIndex(s)
	if loc == nil {
		return 0
	}
	score := weight*10 - loc[0]*3 - len(s)
	if loc[0] == 0 {
		score += weight * 2
	}
	if score < weight {
		score = weight
	}
	return score
}

// fieldScore scores query q (already lowercased) against one field as a
// literal substring; bonuses: prefix, early position, shorter targets.
func fieldScore(q, s string, weight int) int {
	if s == "" || len(q) > len(s) {
		return 0
	}
	ls := strings.ToLower(s)
	idx := strings.Index(ls, q)
	if idx < 0 {
		return 0
	}
	score := weight*10 - idx*3 - len(ls)
	if idx == 0 {
		score += weight * 2
	}
	if score < weight {
		score = weight // any substring match is worth at least the weight
	}
	return score
}
