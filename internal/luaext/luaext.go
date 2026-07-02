// Package luaext hosts the gopher-lua job runtime (ADR-0003): one VM per run,
// instruction budget + wallclock timeout, gluahttp with mandatory timeouts,
// f9 API surface (sessions.*, folders.*, vars.get, log.*). Phase 07.
package luaext
