# ADR-0003: Split into client and daemon binaries

## Status

Accepted. Revises the "single binary" aspect of
[ADR-0002](0002-single-go-binary-with-sqlite.md).

## Context

ADR-0002 chose a single binary with subcommands. But the server and the client
live on different machines with different needs:

- The **client** (`init`, `report`, `tui`) is installed on every machine
  running Claude Code sessions — potentially many, including contributors'.
- The **server** (`serve`) runs on a single host and carries SQLite, an HTTP
  server, and the embedded web dashboard.

Go links every referenced package regardless of use (no dead-code elimination
for a referenced package). A single binary that wires up `serve` therefore
forces every client to ship a dormant HTTP server, SQLite, and web assets.

Startup cost is **not** the driver: Go binaries are `mmap`-ed, so `report`'s
cold start barely depends on binary size. The driver is the deployment boundary
and minimizing the client's surface.

## Decision

Ship **two binaries**:

- **`claude-fleetd`** — the server daemon (`serve`).
- **`claude-fleet`** — the client (`init`, `report`, `tui`).

Shared code lives in `internal/` (`config`, `api`, `version`). The split is
enforced by imports: client packages must not import `internal/server`,
`internal/store`, or `web/`.

The simple name goes to the client — it is what users install everywhere and
invoke by hand. The daemon takes the conventional `d` suffix (sshd, dockerd,
tailscaled).

## Rationale

- Minimal client surface: no dormant server/DB/web on client machines
- Reflects the real, permanent deployment boundary — not a passing convenience
- Follows the established daemon/CLI convention
- Small cost: two build targets in GoReleaser, two archives

## Consequences

### Positive

- The client stays small and focused; server code is isolated to the host
- Clear mental model: `claude-fleetd` serves, `claude-fleet` is what you install

### Negative

- Two artifacts to build, release, and document instead of one
- Shared types must be factored into a neutral package (`internal/api`)

## References

- [ADR-0002](0002-single-go-binary-with-sqlite.md)
- [docs/architecture.md](../architecture.md)
