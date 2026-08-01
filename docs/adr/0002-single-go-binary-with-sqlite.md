# ADR-0002: Single Go binary with SQLite

## Status

Accepted. The "single binary" aspect is superseded by
[ADR-0003](0003-split-client-and-daemon-binaries.md) (two binaries: client +
daemon); the Go / pure-SQLite / GoReleaser decisions still hold.

## Context

Claude Vigie needs a server, a reporter invoked by Claude Code hooks, and two
clients (web + terminal). The reporter is called by hooks — potentially on
every tool use — so its startup cost matters. The project is open source and
self-hosted, so installation must be simple across machines.

## Decision

Ship everything as a **single Go binary** with subcommands (`serve`, `tui`,
`report`, `init`). Persist state in an **embedded SQLite** database file. Embed
the web dashboard assets in the binary via `embed.FS`.

## Rationale

- **Reporter startup**: a native binary starts in a few milliseconds; a
  Node/Python reporter would pay ~100 ms per hook invocation — a visible tax on
  every Claude action.
- **Terminal client**: Bubble Tea (Go) is a first-class TUI toolkit.
- **Distribution**: a static binary cross-compiles for Linux/macOS/Windows and
  needs no runtime installed on the user's machine.
- **Storage**: SQLite is a single file, no database server to deploy — the
  right weight for a self-hosted tool, while still queryable for history.

## Consequences

### Positive

- One artifact to build, ship, and run; trivial self-hosting
- Fast reporter, good terminal UX, full usage history in SQLite

### Negative

- The web dashboard is plain HTML/JS + SSE rather than a rich SPA framework —
  an acceptable trade-off for a monitoring dashboard
- SQLite is single-writer; fine at fleet scale, not built for massive
  multi-tenant write load

## References

- [docs/architecture.md](../architecture.md)
