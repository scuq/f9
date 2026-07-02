# ADR-0006: Store carries revisions from day one

**Status:** accepted · 2026-07-02

## Decision
Every Folder/Session/Var carries `revision` (monotonic) and `updated_at`.
Team sync (phase 09) is deferred, but its data requirements are not: revisions
from day one mean sync is an addition, not a migration. IDs are ULIDs so
distributed creation never collides.
