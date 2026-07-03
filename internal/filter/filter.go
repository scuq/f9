// Package filter is the session fuzzy scorer behind the tree filter box.
// Contract (docs/phase-plan.md 01): ranking 10k sessions must stay under
// 5ms per query — enforced by TestFilterBudget. Dependency-free.
package filter

import (
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

// Rank scores items against query and returns matches, best first (ties by
// name). Empty query returns all items in input order with score 0.
func Rank(query string, items []Item) []Hit {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		out := make([]Hit, len(items))
		for i, it := range items {
			out[i] = Hit{Item: it}
		}
		return out
	}
	out := make([]Hit, 0, 32)
	for _, it := range items {
		s := fieldScore(q, it.Name, weightName)
		if v := fieldScore(q, it.Host, weightHost); v > s {
			s = v
		}
		if v := fieldScore(q, it.Path, weightPath); v > s {
			s = v
		}
		for _, tag := range it.Tags {
			if v := fieldScore(q, tag, weightTag); v > s {
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

// fieldScore scores query q (already lowercased) against one field.
// Exact substring always outranks a scattered subsequence of the same field;
// bonuses: prefix, early position, consecutive runs, shorter targets.
func fieldScore(q, s string, weight int) int {
	if s == "" || len(q) > len(s) {
		return 0
	}
	ls := strings.ToLower(s)

	if idx := strings.Index(ls, q); idx >= 0 {
		score := weight*10 - idx*3 - len(ls)
		if idx == 0 {
			score += weight * 2
		}
		if score < weight {
			score = weight // any substring match is worth at least the weight
		}
		return score
	}

	// Subsequence scan: all query bytes in order, gaps penalized,
	// consecutive matches rewarded.
	j := 0
	prev := -2
	gaps := 0
	bonus := 0
	for i := 0; i < len(q); i++ {
		k := strings.IndexByte(ls[j:], q[i])
		if k < 0 {
			return 0
		}
		abs := j + k
		if abs == prev+1 {
			bonus += 3
		}
		gaps += k
		prev = abs
		j = abs + 1
	}
	score := weight*5 - gaps*2 - len(ls) + bonus
	if score < weight/2 {
		score = weight / 2
	}
	return score
}
