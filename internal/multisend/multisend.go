// Package multisend sends a line or snippet to N marked sessions and tracks a
// per-session feedback state machine: sent -> echoed -> prompt-returned ->
// ok|error-pattern|timeout. Prompt regexes come from osdetect tuning profiles —
// passive matching only. Phase 06.
package multisend

type State string

const (
	StSent           State = "sent"
	StEchoed         State = "echoed"
	StPromptReturned State = "prompt-returned"
	StOK             State = "ok"
	StError          State = "error"
	StTimeout        State = "timeout"
)
