// Package osdetect passively fingerprints the remote OS. It NEVER injects
// bytes: evidence comes from the SSH server version string, login banner,
// prompt shape, pager markers and error idioms observed in the normal data
// flow. Result is persisted to store.SessionMeta and drives per-family tuning
// profiles (configs/os-tunings.yaml). See docs/phase-plan.md 00d.
package osdetect

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

// Detector accumulates passive evidence for one session.
type Detector interface {
	ObserveServerVersion(v string)
	ObserveOutput(p []byte) // banner, prompts, pager markers, error idioms
	Guess() Guess
}

func New() Detector { panic("phase 00d: not implemented") }
