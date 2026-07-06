package store

import (
	"fmt"
	"regexp"
	"strings"
)

// FilterRule is a single leaf condition on a record attribute.
type FilterRule struct {
	Field  string `yaml:"field" json:"field"`             // status|role|hostname|manufacturer|model|tenant|site
	Kind   string `yaml:"kind" json:"kind"`               // eq|contains|regex
	Value  string `yaml:"value" json:"value"`             // literal or regex pattern
	Negate bool   `yaml:"negate,omitempty" json:"negate"` // invert the match
}

// FilterGroup is a boolean group of rules and nested groups. Op defaults to
// "and". An empty group (no rules, no groups) matches everything.
type FilterGroup struct {
	Op     string        `yaml:"op" json:"op"` // and|or
	Rules  []FilterRule  `yaml:"rules,omitempty" json:"rules"`
	Groups []FilterGroup `yaml:"groups,omitempty" json:"groups"`
}

// CompileFilter validates and pre-compiles every regex in the tree once, and
// returns a matcher over a record's attributes. A nil group matches everything.
func CompileFilter(g *FilterGroup) (func(map[string]string) bool, error) {
	if g == nil {
		return func(map[string]string) bool { return true }, nil
	}
	res := map[string]*regexp.Regexp{}
	var walk func(*FilterGroup) error
	walk = func(gr *FilterGroup) error {
		for i := range gr.Rules {
			r := gr.Rules[i]
			if r.Kind == "regex" {
				if _, ok := res[r.Value]; !ok {
					re, err := regexp.Compile(r.Value)
					if err != nil {
						return fmt.Errorf("store: filter regex %q: %w", r.Value, err)
					}
					res[r.Value] = re
				}
			}
		}
		for i := range gr.Groups {
			if err := walk(&gr.Groups[i]); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(g); err != nil {
		return nil, err
	}
	return func(attrs map[string]string) bool { return matchGroup(g, attrs, res) }, nil
}

func matchGroup(g *FilterGroup, attrs map[string]string, res map[string]*regexp.Regexp) bool {
	if len(g.Rules) == 0 && len(g.Groups) == 0 {
		return true
	}
	if g.Op == "or" {
		for i := range g.Rules {
			if matchRule(g.Rules[i], attrs, res) {
				return true
			}
		}
		for i := range g.Groups {
			if matchGroup(&g.Groups[i], attrs, res) {
				return true
			}
		}
		return false
	}
	for i := range g.Rules {
		if !matchRule(g.Rules[i], attrs, res) {
			return false
		}
	}
	for i := range g.Groups {
		if !matchGroup(&g.Groups[i], attrs, res) {
			return false
		}
	}
	return true
}

func matchRule(r FilterRule, attrs map[string]string, res map[string]*regexp.Regexp) bool {
	v := attrs[r.Field]
	var ok bool
	switch r.Kind {
	case "regex":
		if re := res[r.Value]; re != nil {
			ok = re.MatchString(v)
		}
	case "contains":
		ok = strings.Contains(strings.ToLower(v), strings.ToLower(r.Value))
	default: // eq (case-insensitive)
		ok = strings.EqualFold(v, r.Value)
	}
	if r.Negate {
		return !ok
	}
	return ok
}
