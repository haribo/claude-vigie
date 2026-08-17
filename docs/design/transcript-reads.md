# Transcript reads — Design Specification

**Status:** Accepted (#420).

Source of truth for **who reads a session's transcript, and how much of it**. The
watcher's incremental scan is specified here too, because the rule that matters is
the relationship between the two readers, not either one alone.

---

## 1. The defect this replaces

`internal/report` reads the transcript with `transcript.Parse`, which starts at
byte 0. It runs on `Stop` — the end of **every** assistant turn — inside a hook
Claude Code waits on, installed with a 5 s timeout.

Transcripts are append-only and grow for the life of a session, so the cost is
O(file) per turn and only ever increases. Measured with the real parser over the
357 transcripts of one development machine:

| | size | parse |
|---|---|---|
| median | 0.2 MB | ~0 s |
| p90 | 2.3 MB | 0.04 s |
| p99 | 62.5 MB | 1.2 s |
| max | 584.7 MB | **11.1 s** |

At the maximum the hook is killed at its timeout: the report is lost **and** the
operator's session stalls 5 s after every turn.

The size at which this starts to hurt is not a limit anyone chose — it is just
where the file happened to get to. A monitoring tool must not tax the session it
observes ([ADR-0005](../adr/0005-observe-only.md)), and the tax here grows without
bound.

## 2. What the hook actually takes from the transcript

Six fields, on `Stop` and `SessionEnd` only:

| field | how it is derived |
|---|---|
| `Model`, `Effort`, `PermissionMode`, `Title` | last non-empty value wins |
| `ContextTokens` | last main-thread assistant line carrying usage |
| `Usage` | **cumulative** over every assistant message, deduplicated by message id |

Five of the six are answerable from the tail of the file. `Usage` is not: it sums
the whole transcript and needs the set of already-counted message ids to survive
retries. Any scheme that reads less than everything has to carry that set.

## 3. The watcher already reads the transcript correctly

`scanner.parse` (`internal/watch/watch.go`) keeps one `transcript.Parser` per file
and feeds it only the newly-appended bytes, resuming from the parser's offset
(#257). A file that shrank or is seen for the first time is parsed from scratch, so
truncation and rotation are safe. It re-reads nothing.

That scan runs every ~2 s and reports a **superset** of the six fields above.

So on any machine running a watcher, the hook's full re-read produces a value the
server already has, more recently, for a fraction of the work.

## 4. Decision

**A hook reads the transcript only when no live watcher is watching this machine.**

- `vigie watch` marks itself live locally on every heartbeat.
- On `Stop`/`SessionEnd`, `internal/report` reads the transcript only if that mark
  is missing or stale. Otherwise it sends the report without the six fields.
- The server already treats every one of them as *absent → keep the last known*
  (`req.Usage != nil`, `req.ContextTokens != nil`, empty-string guards). No server
  change is required, and a report that omits them cannot erase anything.

A machine with no watcher — explicitly supported by
[ADR-0009](../adr/0009-watcher-managed-hooks.md) for "a one-off machine or a host
that runs no watcher" — keeps today's full parse. It has no other source for these
fields, and its behaviour is unchanged.

## 5. The local mark

`~/.local/state/vigie/watcher`, alongside the existing `sessions/` (presence) and
`compacting/` (compaction markers) state.

- Written by `vigie watch` **after each successful scan**, on the scan interval —
  not on the liveness heartbeat, which is a different clock
  ([watcher-liveness](watcher-liveness.md)). The distinction is deliberate: the
  mark claims *"transcripts on this machine are being read"*, and only a scan
  makes that true.
- **A drifted watcher does not write it.** A watcher whose build no longer matches
  the daemon goes inert and stops scanning ([version-consistency](version-consistency.md)),
  so it stops claiming the mark, and the hooks resume reading transcripts for
  themselves within the window below. The same property as a dead watcher, for the
  same reason.
- **Freshness is the file's mtime**, not its contents; the file body carries the
  watcher's pid for a human reading the directory, and nothing reads it.
- Considered live for 15 s — three missed beats, the same window the TUI already
  applies to a watcher's server-side heartbeat.

The window is what makes the arrangement self-healing: a watcher that dies
mid-session stops beating, the mark goes stale within 15 s, and the hook resumes
reading the transcript on the next turn. Deferring to the watcher is therefore
never a bet that it will stay up.

## 6. Rejected: give the hook its own incremental parser

The obvious symmetry — let the hook resume from an offset like the watcher does —
was rejected.

The watcher can be incremental because it is a **long-lived process** holding the
parser in memory. A hook is a fresh process per event, so it would have to persist
the parser's entire state between runs: the cumulative `Info`, the title tracker,
the pending-tool and pending-agent trackers, and the `seen` set of message ids —
which grows with the session and, on the 585 MB transcript above, holds tens of
thousands of entries.

That is a second copy of the watcher's state, serialized, versioned, and written on
every turn, whose only job is to reproduce a number the watcher already computed
correctly. It trades a bounded cost for an unbounded correctness surface, in the
one process that must never fail the operator's session.

Dropping the `seen` set to make the state small was rejected in turn: a retry
spanning a turn boundary would double-count tokens, and a monitoring tool that
quietly reports wrong numbers is worse than a slow one.

## 7. Consequences

- The cost disappears on the standard setup — the one
  [ADR-0009](../adr/0009-watcher-managed-hooks.md) establishes and the TUI
  preflight already requires.
- `Stop` reports become smaller and constant-time on watched machines.
- The transcript is read by exactly one reader per machine, which is what makes the
  incremental path worth having in the first place.
- A hooks-only machine keeps a cost that grows with session length. That is
  unchanged, now written down, and the reason to run a watcher.
