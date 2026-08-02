# ADR-0004: When to split into a separate binary

## Status

Accepted. Builds on [ADR-0003](0003-split-client-and-daemon-binaries.md).

## Context

As the project grows (watcher, terminal UI, a possible GTK4 desktop GUI), the
same question keeps coming up: should this concern be a **new binary** or a
**subcommand** of an existing one? Without a rule, we drift toward one binary
per component, which fragments shared code and distribution for no benefit.

Four levels are easy to conflate and must be kept distinct:

| Level | Example | Named for |
|-------|---------|-----------|
| Binary (distributed artifact) | `vigie`, `vigied` | this ADR |
| Subcommand | `vigie watch`, `vigie tui` | git/docker style |
| justfile recipe | `app-serve`, `app-tui` | dev/ops convenience |
| systemd service | `vigie-watch.service` | an execution mode |

## Decision

Split a concern into its **own binary** only when one of these holds:

1. **Heavy dependencies** we don't want everywhere (e.g. GTK4/CGO), or
2. A **deployment boundary** — it runs on a different machine.

Otherwise it is a **subcommand** of the relevant binary.

Applied:

- **`vigied`** — the server. Separate: deployment boundary (central host).
- **`vigie`** — the client CLI (`init`, `report`, `watch`, `tui`). One
  binary, subcommands: they share the client code (config, transcript, api), run
  on the same machine, same role.
- **`vigie-gui`** (future) — a GTK4 desktop app. Separate: heavy CGO/GTK
  dependencies that must not bloat the lightweight CLI.

The short name (`vigie`) belongs to the client CLI — the one humans type
most (per ADR-0003). A new binary does **not** take the short name.

## Rationale

Multiplying binaries by "component type" (a binary for the watcher, one for the
tui…) fragments shared code and produces more artifacts with no upside. What
actually warrants a separate binary is a **constraint**: dependency weight or a
deployment boundary. `watch` and `tui` have neither; a GTK4 GUI has the first.

## Consequences

### Positive

- A clear, repeatable rule; the CLI stays light; a future GUI won't pollute it.
- Recipe/service names stay free to be convenient (e.g. the `app-` prefix),
  without being mistaken for binaries.

### Negative

- The client binary bundles several subcommands — acceptable, since they share
  code and ship as one artifact.

## References

- [ADR-0003](0003-split-client-and-daemon-binaries.md)
- [docs/architecture.md](../architecture.md)
