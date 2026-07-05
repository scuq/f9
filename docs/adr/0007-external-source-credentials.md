# ADR-0007: External-source credentials (encrypted at rest)

**Status:** accepted · 2026-07-05

## Context
Phase 07 imports sessions from external HTTPS sources configured per folder.
Calling those endpoints needs auth (bearer / HTTP basic / mTLS). Unlike SSH auth
(ADR-0005, never persisted), these credentials must survive restarts.

## Decision
- ADR-0005 is scoped to SSH/session dialing: SSH passwords and key passphrases
  are still never persisted; imported sessions dial via that same auth order.
- External-source credentials live in a separate, passphrase-locked store
  (`internal/cred`): a key is derived from a user passphrase (argon2id) and each
  secret is sealed with NaCl secretbox. The passphrase is never persisted; the
  store opens locked and is unlocked on demand.
- Storage is a single `<root>/.secrets.yaml` (salt + a check blob + sealed
  items), mode 0600, atomic writes. Nothing readable without the passphrase.

## Consequences
- A forgotten passphrase means re-entering source credentials (acceptable).
- The session store itself stays non-confidential (ADR-0005 era); only the
  separate `.secrets.yaml` holds ciphertext.
- No new dependency: argon2id and secretbox come from the existing
  `golang.org/x/crypto`.
