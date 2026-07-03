// Package snippets: tree-organized command snippets with jinja2-style
// templating (pongo2), variable resolution from the vars store, prompt-for-
// unresolved at paste time, and paste modes including per-line delay for device
// config pastes. This file is the rendering engine (phase 05b); the snippet
// store tree and the timed send land in phase 05c.
package snippets

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/flosch/pongo2/v6"
)

// Snippet is one templated command block in the snippet tree.
type Snippet struct {
	ID       string
	FolderID string
	Name     string
	Body     string // pongo2 template; multiline
}

// PasteMode controls how a rendered snippet is sent to a terminal.
type PasteMode struct {
	LineDelayMs int  // e.g. 100 for Cisco config paste (typed line-by-line)
	Bracketed   bool // wrap in bracketed-paste for shells that support it
}

// Render executes body as a pongo2 template with the given variables. Missing
// variables render as empty (pongo2 default); call Unresolved first to prompt.
func Render(body string, vars map[string]string) (string, error) {
	tpl, err := pongo2.FromString(body)
	if err != nil {
		return "", fmt.Errorf("snippets: parse: %w", err)
	}
	ctx := pongo2.Context{}
	for k, v := range vars {
		ctx[k] = v
	}
	out, err := tpl.Execute(ctx)
	if err != nil {
		return "", fmt.Errorf("snippets: render: %w", err)
	}
	return out, nil
}

// Unresolved returns the required variables (see RequiredVars) that are absent
// from vars — the set to prompt the user for before pasting. Sorted.
func Unresolved(body string, vars map[string]string) []string {
	var out []string
	for _, name := range RequiredVars(body) {
		if _, ok := vars[name]; !ok {
			out = append(out, name)
		}
	}
	return out
}

// --- required-variable extraction (best-effort) ---
//
// pongo2 exposes no "list template variables" API, so RequiredVars is a
// heuristic over the template text. It recognizes {{ ... }} output tags and
// {% if/elif ... %} / {% for v in expr %} control tags, extracting the root
// identifiers while excluding: pongo2 keywords, loop variables, filter names
// (after '|'), attribute accessors (after '.'), and string literals. Deeply
// nested or exotic expressions may under-report; the paste-time prompt is
// therefore additive, never a hard gate.

var (
	exprRe   = regexp.MustCompile(`\{\{(.*?)\}\}`)
	ctrlRe   = regexp.MustCompile(`\{%(.*?)%\}`)
	identRe  = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
	strLitRe = regexp.MustCompile(`"[^"]*"|'[^']*'`)
	forRe    = regexp.MustCompile(`^for\s+([A-Za-z_][\w,\s]*?)\s+in\s+(.+)$`)
	reserved = map[string]bool{}
)

func init() {
	for _, k := range []string{
		"true", "false", "none", "null", "not", "and", "or", "in", "is",
		"if", "elif", "else", "endif", "for", "endfor", "block", "endblock",
		"set", "with", "endwith", "range", "loop",
	} {
		reserved[k] = true
	}
}

// RequiredVars returns the distinct root variables a template references,
// sorted. See the package note above for the heuristic's limits.
func RequiredVars(body string) []string {
	seen := map[string]bool{}
	provided := map[string]bool{} // loop variables
	var req []string

	// Pass 1: control tags — collect loop vars, and requireds from if/for exprs.
	for _, m := range ctrlRe.FindAllStringSubmatch(body, -1) {
		inner := strings.TrimSpace(m[1])
		fields := strings.Fields(inner)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "for":
			if fm := forRe.FindStringSubmatch(inner); fm != nil {
				for _, lv := range strings.Split(fm[1], ",") {
					if lv = strings.TrimSpace(lv); lv != "" {
						provided[lv] = true
					}
				}
				collectIdents(fm[2], &req, seen, provided)
			}
		case "if", "elif":
			collectIdents(strings.TrimSpace(strings.TrimPrefix(inner, fields[0])), &req, seen, provided)
		}
	}
	// Pass 2: output tags.
	for _, m := range exprRe.FindAllStringSubmatch(body, -1) {
		collectIdents(m[1], &req, seen, provided)
	}

	sort.Strings(req)
	return req
}

func collectIdents(expr string, req *[]string, seen, provided map[string]bool) {
	expr = strLitRe.ReplaceAllString(expr, " ") // drop string literals
	for _, loc := range identRe.FindAllStringIndex(expr, -1) {
		if loc[0] > 0 {
			if prev := expr[loc[0]-1]; prev == '.' || prev == '|' {
				continue // attribute accessor or filter name
			}
		}
		name := expr[loc[0]:loc[1]]
		if reserved[name] || provided[name] || seen[name] {
			continue
		}
		seen[name] = true
		*req = append(*req, name)
	}
}

// --- paste helpers ---

const (
	bracketStart = "\x1b[200~"
	bracketEnd   = "\x1b[201~"
)

// BracketedWrap wraps s in bracketed-paste markers so a supporting shell treats
// it as pasted (literal) input rather than typed commands.
func BracketedWrap(s string) string { return bracketStart + s + bracketEnd }
