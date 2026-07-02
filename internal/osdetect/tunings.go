package osdetect

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// Tuning is one family's profile from configs/os-tunings.yaml. Profiles are
// data, not code: consumers are the multi-send feedback matcher (prompt/error
// regex, phase 06), send-newline style, and default highlight sets (phase 04).
type Tuning struct {
	PromptRegex       string   `yaml:"prompt_regex"`
	ErrorRegex        string   `yaml:"error_regex"`
	PagerRegex        string   `yaml:"pager_regex"`
	Newline           string   `yaml:"newline"`
	DefaultHighlights []string `yaml:"default_highlights"`
}

// Compile validates and compiles the tuning's regexes.
func (t Tuning) Compile() (prompt, errRe, pager *regexp.Regexp, err error) {
	if t.PromptRegex != "" {
		if prompt, err = regexp.Compile(t.PromptRegex); err != nil {
			return nil, nil, nil, fmt.Errorf("osdetect: prompt_regex: %w", err)
		}
	}
	if t.ErrorRegex != "" {
		if errRe, err = regexp.Compile(t.ErrorRegex); err != nil {
			return nil, nil, nil, fmt.Errorf("osdetect: error_regex: %w", err)
		}
	}
	if t.PagerRegex != "" {
		if pager, err = regexp.Compile(t.PagerRegex); err != nil {
			return nil, nil, nil, fmt.Errorf("osdetect: pager_regex: %w", err)
		}
	}
	return prompt, errRe, pager, nil
}

// LoadTunings reads and validates configs/os-tunings.yaml.
func LoadTunings(path string) (map[Family]Tuning, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("osdetect: read tunings: %w", err)
	}
	var doc struct {
		Families map[string]Tuning `yaml:"families"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("osdetect: parse tunings: %w", err)
	}
	out := make(map[Family]Tuning, len(doc.Families))
	for name, t := range doc.Families {
		if _, _, _, err := t.Compile(); err != nil {
			return nil, fmt.Errorf("osdetect: family %s: %w", name, err)
		}
		out[Family(name)] = t
	}
	return out, nil
}
