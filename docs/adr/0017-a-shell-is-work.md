# ADR-0017: A shell running for Claude is work

## Status

Accepted (#851).

## Context

Claude Code's session registry carries a `status` of `busy`, `shell`, `idle` or
`waiting`. #254 read the enum from the binary, left `shell → ?` explicitly *"to
define"*, and leaned `working`. #255 — the implementation, opened nine minutes
later with the work already done — mapped it to `idle` instead, on one line:

> **`shell → idle`** (not working): the user is at a bash shell, Claude is not
> producing.

That sentence was never checked against a running session, and nothing else
recorded it: #255 carries no comment, and the commit that applied it (`f98312b`)
touched no design document. The decision existed only in `registryStatuses`.

**Measured on real sessions.** Claude Code v2.1.267, driven through a pty, the
registry and the board sampled every 10 s:

| what the session does | registry | vigie showed |
|---|---|---|
| `!` — operator at the bash prompt, `sleep 90` | `busy` | `idle` |
| `Monitor` — shell run for Claude, `sleep 90` | `shell` | `idle`, DETAIL `shell` |

The operator at a `!` prompt — the entire justification for `idle` — reports
`busy`. `shell` appears while a shell runs *for Claude*.

Four bugs patched around this line rather than reopening it: #661 (a running
command reads idle, then stalled), #748, #834, #842. Each added a transcript-side
exception to recover `working` for one shape of work, and each was defeated by the
next shape — a `Monitor` answered in 1.8 s leaves nothing pending for an exception
to see.

## Decision

**`shell` maps to `working`.**

DETAIL keeps the word (#280), as a fallback: when the transcript has something more
precise — which tool is running, or that the work is a backgrounded command — that
wins.

## Consequences

- The operator at a `!` prompt is unaffected: that state reports `busy`, which was
  already `working`.
- **No `working` can latch.** The registry gives both edges: it enters `shell` when
  the command starts and leaves it when the command ends — measured at t=101 s for a
  90 s command. Nothing is inferred to close it, so [ADR-0015](0015-no-timer-decides-what-vigie-cannot-observe.md)
  is satisfied by observation rather than by a timer.
- The transcript-side exceptions built to recover `working` from `shell` (#661) are
  now redundant on this path. They are not removed here: they still carry the DETAIL
  precedence, and they remain the only route for a client the registry does not
  cover.
- **If a future Claude Code changed what `shell` means, this would be wrong again.**
  The guard is not a test — a test only ever sees the word we wrote down
  ([ADR-0016](0016-capture-claude-code-artefacts.md)) — but a measurement against a
  real session, of the kind that produced the table above. The table carries its
  version and its date for that reason, and is never carried forward.
