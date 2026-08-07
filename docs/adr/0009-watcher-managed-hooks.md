# ADR-0009: The watcher owns the reporting-hooks lifecycle

## Status

Accepted.

## Context

Reporting hooks are installed into `~/.claude/settings.json` only by an explicit
`vigie init` or `vigie hooks install`, and nothing detects staleness:

- an install predating a release silently lacks a newer hook (e.g. `PreCompact`,
  added in 0.3.0 — [ADR-0008](0008-compacting-status.md));
- a moved or upgraded binary leaves a dead `binPath` in the hook commands;
- so the installed hooks and the running `vigie` binary can silently disagree.

Two writers touch `settings.json` — vigie and Claude Code itself. The current
write is `os.WriteFile` (whole file, no atomicity): a crash or a concurrent read
can see a torn file, and there is no single point that keeps hooks current.

This also blocks the fleet-consistency work ([#356](https://github.com/haribo/claude-vigie/issues/356)/[#359](https://github.com/haribo/claude-vigie/issues/359)):
a TUI preflight wants to assert "the local watcher is current", but without a
guaranteed link between the watcher version and the installed hooks that check
would have to inspect both independently.

## Decision

**`vigie watch` owns the hooks lifecycle, and the settings write is atomic.**

- **Refresh at startup.** `vigie watch` (re)installs its own leg into
  `settings.json` before it starts scanning. Hooks therefore match the running
  watcher's binary and default event set **by construction** — a service restart
  after an upgrade self-heals stale hooks. This makes *watcher version == hooks
  version*, so a single check downstream ([#356](https://github.com/haribo/claude-vigie/issues/356)/[#359](https://github.com/haribo/claude-vigie/issues/359))
  covers both.
- **Manual commands stay.** `vigie init`, `vigie hooks install`, and
  `vigie hooks uninstall` are unchanged — for a one-off machine or a host that
  runs no watcher.
- **Scoped to vigie's own leg.** The merge only replaces the matchers of the
  leg identified by `VIGIE_CONFIG` (production when unset); foreign top-level
  settings keys and foreign hooks are preserved.
- **Atomic write.** Settings are written to a temp file in the same directory
  and `rename`d over the target — a reader never sees a partial file. (This does
  not add cross-process locking; a genuinely concurrent write is still
  last-writer-wins, but never torn.)
- **Best-effort, never fatal.** A refresh failure — an unreadable or malformed
  `settings.json` — is logged to stderr (the systemd journal) and the watcher
  keeps observing. A hooks problem must not stop the watch.

## Consequences

- **The install contract changes: explicit → watcher-managed.** Documented in
  [`architecture.md`](../architecture.md). A machine running the watcher as a
  service no longer needs a manual `vigie init` after an upgrade to pick up new
  hooks.
- **No torn settings file.** The atomic rename removes the partial-write failure
  mode; it does not serialize vigie against Claude Code's own writes.
- **Observe-only holds** ([ADR-0005](0005-observe-only.md)): installing a
  reporting hook is client-side configuration of Claude Code's own hook surface,
  not a control channel into a session.
