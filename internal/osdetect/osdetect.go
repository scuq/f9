// Package osdetect passively fingerprints the remote OS. It NEVER injects
// bytes: evidence comes from the SSH server version string, login banner,
// prompt shape, pager markers and error idioms observed in the normal data
// flow. Result is persisted to store.SessionMeta and drives per-family tuning
// profiles (configs/os-tunings.yaml). See docs/phase-plan.md 00d.
package osdetect

import (
	"bytes"
	"sync"
)

type Family string

const (
	FamilyUnknown Family = "unknown"
	FamilyLinux   Family = "linux"
	FamilyOpenBSD Family = "openbsd"
	FamilyIOS     Family = "ios"
	FamilyNXOS    Family = "nxos"
	FamilyPANOS   Family = "panos"
	FamilyJunos   Family = "junos"
	FamilyWindows Family = "windows"
)

type Guess struct {
	Family     Family
	Confidence float64 // 0..1; persist once above threshold
}

// DefaultThreshold is the confidence above which a guess is written to
// SessionMeta (unless the user pinned an override).
const DefaultThreshold = 0.75

const (
	observeBudget = 256 << 10 // stop scanning after this many bytes
	maxTail       = 512       // carried partial-line cap
)

// Detector accumulates passive evidence for one session. Safe for use from
// the OnData fan-out goroutine concurrently with Guess() callers.
type Detector interface {
	ObserveServerVersion(v string)
	ObserveOutput(p []byte) // banner, prompts, pager markers, error idioms
	Guess() Guess
}

type detector struct {
	mu       sync.Mutex
	scores   map[Family]float64
	tail     []byte
	observed int
	done     bool

	firedVersion []bool
	firedLine    []bool
	firedPrompt  []bool

	// relay: the byte stream passes through a unix jumphost (shell-hop), so
	// host-banner evidence must be ignored — it describes the hop.
	relay bool
}

func New() Detector {
	return &detector{
		scores:       map[Family]float64{},
		firedVersion: make([]bool, len(versionRules)),
		firedLine:    make([]bool, len(lineRules)),
		firedPrompt:  make([]bool, len(promptRules)),
	}
}

// NewRelay returns a detector for sessions whose stream passes through a
// unix jumphost (shell-hop): unix host-banner line rules are ignored so the
// hop's motd cannot label the target. Device-idiom evidence (prompts, error
// strings, pagers, product banners) still applies.
func NewRelay() Detector {
	d := New().(*detector)
	d.relay = true
	return d
}

func (d *detector) ObserveServerVersion(v string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	b := []byte(v)
	for i, r := range versionRules {
		if d.firedVersion[i] || !bytes.Contains(b, r.hint) {
			continue
		}
		d.firedVersion[i] = true
		d.apply(r)
	}
}

// ObserveOutput scans p line by line (carrying partial lines across calls)
// against banner/error/pager rules, then matches the current tail — the
// incomplete line where a prompt would sit — against prompt rules.
// After observeBudget bytes it becomes a cheap no-op forever.
func (d *detector) ObserveOutput(p []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.done {
		return
	}
	d.observed += len(p)

	data := p
	if len(d.tail) > 0 {
		data = append(d.tail, p...)
	}
	for {
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			break
		}
		d.matchLine(bytes.TrimRight(data[:idx], "\r "))
		data = data[idx+1:]
	}
	if len(data) > maxTail {
		data = data[len(data)-maxTail:]
	}
	d.tail = append([]byte(nil), data...)

	d.matchPrompt(bytes.TrimRight(stripANSI(d.tail), " "))

	if d.observed > observeBudget {
		d.done = true
		d.tail = nil
	}
}

func (d *detector) matchLine(line []byte) {
	if len(line) == 0 {
		return
	}
	for i, r := range lineRules {
		if d.relay && r.hostBanner {
			continue
		}
		if d.firedLine[i] || !bytes.Contains(line, r.hint) {
			continue
		}
		d.firedLine[i] = true
		d.apply(r)
	}
}

func (d *detector) matchPrompt(tail []byte) {
	if len(tail) == 0 {
		return
	}
	for i, r := range promptRules {
		if d.firedPrompt[i] || !r.re.Match(tail) {
			continue
		}
		d.firedPrompt[i] = true
		d.apply(r)
	}
}

func (d *detector) apply(r rule) {
	for _, fw := range r.weights {
		d.scores[fw.fam] += fw.w
	}
}

// Guess returns the current best guess. Confidence is the top family's share
// of all evidence, dampened when total evidence is thin:
//
//	confidence = (top / total) * min(1, top/4)
//
// so one weak hit can never look certain.
func (d *detector) Guess() Guess {
	d.mu.Lock()
	defer d.mu.Unlock()
	var top, total float64
	var topFam Family
	for fam, s := range d.scores {
		total += s
		if s > top || (s == top && (topFam == "" || fam < topFam)) {
			top, topFam = s, fam
		}
	}
	if topFam == "" || total == 0 {
		return Guess{Family: FamilyUnknown}
	}
	damp := top / 4
	if damp > 1 {
		damp = 1
	}
	return Guess{Family: topFam, Confidence: top / total * damp}
}

// stripANSI removes CSI escape sequences (ESC [ ... final), lone ESC pairs
// and carriage returns, so colored prompts still match the prompt rules.
func stripANSI(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c == 0x1b {
			if i+1 < len(b) && b[i+1] == '[' {
				j := i + 2
				for j < len(b) && (b[j] < 0x40 || b[j] > 0x7e) {
					j++
				}
				i = j // also skips the final byte
				continue
			}
			i++ // ESC + one char (e.g. charset selection)
			continue
		}
		if c == '\r' {
			continue
		}
		out = append(out, c)
	}
	return out
}
