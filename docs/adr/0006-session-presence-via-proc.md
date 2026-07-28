# ADR-0006: Detect session liveness via a hook-declared PID and /proc

## Status

Accepted.

## Context

The watcher derives a session's status from how recently its transcript changed
(see [`docs/design/session-status.md`](../design/session-status.md)). Activity is
enough to tell `working` from *not working*, but not `idle` (alive, between
turns) from `ended` (closed): both are simply *quiet transcripts*. Without a
liveness signal the watcher must guess — and a session idle for an hour looks
exactly like one that was closed an hour ago.

We need to answer, reliably, "is the process behind this session still running?"

**Passive detection does not work.** From the outside there is nothing on the
`claude` OS process that carries the Claude *session id*: we cannot scan the
process table and map a running process back to a session. Matching on cwd,
command line, or transcript path is heuristic and breaks with multiple sessions
per directory, renamed transcripts, or reused working dirs. The link between a
session id and its process has to be captured from *inside* the session, where
both are known.

## Decision

**Record the session→process link at a hook, and check liveness through
`/proc`.**

- **Capture (hook).** A hook runs as a *descendant* of the `claude` process and
  knows the session id. It walks its own `/proc` ancestor chain (`ppid` by
  `ppid`) up to the nearest process whose `comm` is `claude`, and records that
  process's `{pid, start_time}` as the session's mapping.
- **Store.** Mappings live at `~/.local/state/claude-fleet/sessions/<id>.json`.
  The path is derived purely from `HOME` — **not** `XDG_STATE_HOME` — so the
  hook's environment and the watcher's systemd environment always resolve to the
  same directory.
- **Liveness (watcher).** A session is alive iff `/proc/<pid>` still exists **and**
  its `start_time` is unchanged. The `start_time` guard defeats pid reuse: a
  different process that inherits the same pid has a different start time, so a
  dead session is never mistaken for a live one.
- **Lifecycle.** The mapping is saved at `SessionStart`, **backfilled** at
  `UserPromptSubmit` (a session already open when the hook was installed never
  replays `SessionStart`, so its next message registers it), deleted at
  `SessionEnd`, and garbage-collected when the process is dead and the mapping
  file has gone stale.

## Consequences

- **Reliable idle vs ended.** An alive-but-quiet session reads as `idle` for any
  duration; a session whose process is gone reads as `ended` — even on a hard
  kill that skipped `SessionEnd`. This is the presence input to the watcher's
  `statusFor` logic.
- **Linux only.** Reading `/proc` ties this to Linux. Presence is an
  *enhancement*: where it is unavailable (non-Linux, or a hook not run under
  Claude Code), capture no-ops and the watcher falls back to transcript-only
  heuristics — a session with no mapping and no activity is presumed `ended`.
- **Hooks must not fail the session.** Presence capture is best-effort; the hook
  ignores its errors and always exits 0, so a fleet problem never blocks Claude.
- **Observe-only.** Reading `/proc` and a mapping file is pure observation,
  consistent with [ADR-0005](0005-observe-only.md); nothing is written into the
  session.
