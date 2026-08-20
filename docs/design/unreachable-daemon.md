# Unreachable daemon — Design Specification

**Status:** Accepted (#578).

Source of truth for **what a Claude Code hook does when the daemon does not
answer**. The watcher's part is specified here too, because the rule that matters
is the relationship between the two — the same shape as
[transcript-reads](transcript-reads.md), for the same reason.

---

## 1. The defect this replaces

`internal/report` POSTs synchronously and remembers nothing between runs. The
hooks installed by default include `PostToolUse` (`internal/client/hooks.go`), so
the POST happens after **every** tool call, and `report.go` waits up to 3 s for a
reply. Nothing changes between attempts: a daemon that stopped answering costs the
full deadline every time, for as long as it stays down.

Measured against a server that accepts the connection and never answers, three
`PostToolUse` events in a row:

| event | elapsed |
|---|---|
| #1 | 3.00 s |
| #2 | 3.00 s |
| #3 | 3.00 s |

Three events, three connections, 9.01 s. A turn doing 80 tool calls loses about
four minutes.

A daemon *stopped* on the same host is harmless — `connection refused` returns
immediately. The case that costs is the one that black-holes packets: a host that
suspended, a VPN that dropped, a firewall that started discarding.

The operator is told nothing. The error goes to stderr
(`internal/client/report.go`), which lands in a hook debug log nobody reads. It
looks like Claude got slow.

This is the same defect as [transcript-reads](transcript-reads.md) § 1 with a
different meter — there the cost grew with the transcript, here it repeats per
event — and it fails the same principle: **a monitoring tool must not tax the
session it observes** ([ADR-0005](../adr/0005-observe-only.md)).

## 2. Decision

**A hook does not POST while the daemon is known to be unreachable.**

- Reachability is recorded on the machine itself, as a mark with an mtime.
- `internal/report` reads it first. A fresh mark means: return immediately,
  opening no connection.
- A stale or missing mark means: POST as today. That attempt is what re-probes,
  so recovery needs no operator action.
- A **transport** failure arms the mark. An HTTP error *response* does not — the
  daemon answered, so it is reachable; the report was refused for its content
  (drift, validation), which is a different subject with its own handling
  ([version-consistency](version-consistency.md)).

## 3. Both processes write the mark

The watcher talks to the daemon constantly — a heartbeat every 5 s, a report per
session per scan, the usage lease — and **every one of those requests maintains
the mark**, at the transport helpers they all pass through.

That is what makes the arrangement cost nothing on a normal machine: the watcher
finds the failure long before any hook does, and no tool call ever pays the
deadline. The hook writes the mark too, so a machine running no watcher —
explicitly supported by [ADR-0009](../adr/0009-watcher-managed-hooks.md) — still
stops after its first victim rather than paying on every event.

**Not on the heartbeat alone**, which was the first design and is wrong. The beat
is not reliably every 5 s: during an outage the watcher waits out a deadline for
each session report before coming back round to beating, so a machine with a
dozen live sessions can take longer than § 4's window to beat again. The mark
would expire between two beats and hooks would resume paying. Refreshing it on
every request makes it independent of how long a scan cycle happens to take.

The watcher never *reads* the mark. It is long-lived, so waiting out a deadline
delays nobody, and it has to keep probing — otherwise nothing would ever clear
the mark.

One file, two writers, one reader. The hook-writes / watcher-reads split of
[ADR-0006](../adr/0006-session-presence-via-proc.md) and
[compaction](../adr/0008-compacting-status.md), with the direction relaxed rather
than a second mechanism added.

The two writers cannot deadlock the state: whoever last talked to the daemon wins,
and both write the same fact. A hook whose POST fails transiently while the
watcher's beat succeeds arms the mark for at most one beat — correct, since the
daemon is in fact reachable.

## 4. The mark is per server, not per machine

`~/.local/state/vigie/unreachable/<key>`, where `<key>` derives from the
configured server URL. Alongside the existing `sessions/` (presence),
`compacting/` (compaction markers) and `watcher` (local watcher mark).

A single mark per machine would be wrong, and wrong on a setup this repository
ships: `vigie hooks` installs **one leg per `VIGIE_CONFIG`** (README), and the
justfile runs a development server on `:8099` beside a production client. Stopping
the development daemon — `just dev-down`, routine — would suppress production
hook reports for the whole window. The two legs are two daemons whose
reachability is independent, so their marks are.

- **Freshness is the file's mtime**, not its contents; the body carries the server
  URL and the error for a human reading the state directory, and nothing parses it.
- **Every failure to read the mark answers "reachable"**. The fallback is to
  attempt the POST, so an unreadable mark must never be read as "do not bother" —
  the mirror of [transcript-reads](transcript-reads.md) § 5, where an unreadable
  mark must never be read as "someone else has it covered". In both cases the
  unknown answer is the one that does the work.
- Held for **60 s**. The window is bounded by what it costs to be wrong in each
  direction, and both are small: too long delays the return of hook-only signals
  after recovery, too short pays the deadline again. 60 s is twelve watcher beats
  — a watcher that is running clears the mark long before it expires, so the value
  only governs the hooks-only machine.

## 5. What is lost while the mark is armed

Only what a hook alone can see. The watcher reports a superset of the session
fields every ~2 s, so status, tokens and activity are unaffected — except that the
watcher is failing to reach the same daemon, so nothing is arriving anyway. That
is the point: the reports being skipped are reports that would not have landed.

The signal that is hook-only is `waiting`, which rides the `Notification` event.
A session that starts waiting during an outage is not shown as waiting until the
first hook after the mark expires. An outage already blinds the board; this does
not blind it further.

## 6. Rejected: send the report in the background

Detaching the POST into a background process would remove the wait without any
mark. It was rejected: it spawns a process per tool call, reorders reports against
each other, and leaves nothing to observe when it fails. The cost it removes is
the same cost a mark removes, for a far larger surface, in the one process that
must never fail the operator's session — the same trade
[transcript-reads](transcript-reads.md) § 6 refused for the same reason.

## 7. Consequences

- On the standard setup — a watcher, per [ADR-0009](../adr/0009-watcher-managed-hooks.md)
  — the cost disappears entirely: no hook ever waits on a dead daemon.
- On a hooks-only machine it is paid once per window instead of once per event.
- An outage stops being something the operator feels as latency in a session and
  cannot explain.
- The mark is one more piece of local state whose staleness must be self-healing;
  like the watcher mark, its window is what guarantees that.
