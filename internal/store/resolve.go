package store

// overlay returns base with every non-nil field of o applied on top.
// A nil pointer (or nil JumpChain slice) means "inherit". This implements the
// SecureCRT-style "Default Session (root folder) -> folder chain -> session"
// semantics. When SessionOptions grows a field, extend this function — the
// field-count guard in resolve_test.go will fail until you do.
func overlay(base, o SessionOptions) SessionOptions {
	if o.TermType != nil {
		base.TermType = o.TermType
	}
	if o.KeepaliveInterval != nil {
		base.KeepaliveInterval = o.KeepaliveInterval
	}
	if o.Reconnect != nil {
		base.Reconnect = o.Reconnect
	}
	if o.ThemeRef != nil {
		base.ThemeRef = o.ThemeRef
	}
	if o.JumpChain != nil {
		base.JumpChain = o.JumpChain
	}
	if o.ScrollbackLines != nil {
		base.ScrollbackLines = o.ScrollbackLines
	}
	if o.AuditScope != nil {
		base.AuditScope = o.AuditScope
	}
	if o.KeyFile != nil {
		base.KeyFile = o.KeyFile
	}
	if o.UseAgent != nil {
		base.UseAgent = o.UseAgent
	}
	return base
}
