package app

import (
	"fmt"
	"regexp"

	"github.com/scuq/f9/internal/scrollback"
)

// grepHardCap bounds how many matches cross the Wails boundary at once so a
// broad pattern over millions of lines can't flood the UI.
const grepHardCap = 5000

type GrepOptsDTO struct {
	Invert     bool `json:"invert"`
	IgnoreCase bool `json:"ignoreCase"`
	Before     int  `json:"before"`
	After      int  `json:"after"`
	MaxMatches int  `json:"maxMatches"`
}

type GrepMatchDTO struct {
	LineNo int      `json:"lineNo"` // 1-based for display
	Line   string   `json:"line"`
	Before []string `json:"before"`
	After  []string `json:"after"`
}

type GrepResultDTO struct {
	Matches   []GrepMatchDTO `json:"matches"`
	Count     int            `json:"count"`
	Truncated bool           `json:"truncated"`
	Lines     int            `json:"lines"` // total scrollback lines searched
}

// GrepTerminal greps a live terminal's full scrollback history.
func (a *App) GrepTerminal(termID, pattern string, opts GrepOptsDTO) (*GrepResultDTO, error) {
	if pattern == "" {
		return nil, fmt.Errorf("app: empty pattern")
	}
	a.tmu.Lock()
	t, ok := a.terms[termID]
	a.tmu.Unlock()
	if !ok {
		return nil, fmt.Errorf("app: terminal not open")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("app: pattern: %w", err)
	}
	limit := opts.MaxMatches
	if limit <= 0 || limit > grepHardCap {
		limit = grepHardCap
	}
	it, err := t.sb.Grep(re, scrollback.GrepOpts{
		Invert:     opts.Invert,
		IgnoreCase: opts.IgnoreCase,
		Before:     opts.Before,
		After:      opts.After,
		MaxMatches: limit + 1, // one extra to detect truncation
	})
	if err != nil {
		return nil, err
	}
	defer it.Close()

	lines, _ := t.sb.Len()
	res := &GrepResultDTO{Lines: lines}
	for {
		m, ok := it.Next()
		if !ok {
			break
		}
		if len(res.Matches) >= limit {
			res.Truncated = true
			break
		}
		res.Matches = append(res.Matches, GrepMatchDTO{
			LineNo: m.LineNo + 1,
			Line:   string(m.Line),
			Before: bytesToStrings(m.Before),
			After:  bytesToStrings(m.After),
		})
	}
	res.Count = len(res.Matches)
	return res, nil
}

// PeekDTO is a window of scrollback lines with the 1-based line number of its
// first line.
type PeekDTO struct {
	Start int      `json:"start"`
	Lines []string `json:"lines"`
}

// TerminalPeek returns context lines around a 0-based absolute line number,
// clamped to the retained window. Used to expand a search result in place.
func (a *App) TerminalPeek(termID string, lineNo0, context int) (*PeekDTO, error) {
	a.tmu.Lock()
	t, ok := a.terms[termID]
	a.tmu.Unlock()
	if !ok {
		return nil, fmt.Errorf("app: terminal not open")
	}
	if context < 0 {
		context = 0
	}
	if context > 200 {
		context = 200
	}
	first := t.sb.FirstLine()
	total, _ := t.sb.Len()
	end := first + total
	from := lineNo0 - context
	if from < first {
		from = first
	}
	to := lineNo0 + context + 1
	if to > end {
		to = end
	}
	if from >= to {
		return &PeekDTO{Start: from + 1}, nil
	}
	raw, err := t.sb.Lines(from, to)
	if err != nil {
		return nil, err
	}
	return &PeekDTO{Start: from + 1, Lines: bytesToStrings(raw)}, nil
}

// TerminalStats returns the number of lines currently in a terminal's scrollback.
func (a *App) TerminalStats(termID string) (int, error) {
	a.tmu.Lock()
	t, ok := a.terms[termID]
	a.tmu.Unlock()
	if !ok {
		return 0, fmt.Errorf("app: terminal not open")
	}
	lines, _ := t.sb.Len()
	return lines, nil
}

func bytesToStrings(b [][]byte) []string {
	if len(b) == 0 {
		return nil
	}
	out := make([]string, len(b))
	for i, x := range b {
		out[i] = string(x)
	}
	return out
}
