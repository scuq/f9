# ADR-0005: No password storage

**Status:** accepted · 2026-07-02

## Decision
f9 never persists passwords or key passphrases. Auth order: ssh-agent -> key
files (passphrase via prompt callback) -> keyboard-interactive/password prompt.
Prompt results live only for the dialing attempt. The vars store rejects
secret-like keys. Session store therefore contains nothing confidential, which
also simplifies team sync (ADR-0006 era).
