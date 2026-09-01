# Status time — Design Specification

**Status:** Accepted (#668).

Source of truth for the durations the Stats tab reports — how long the fleet spent
`working`, `waiting` and `idle`. Token totals are a different rollup with a
different source; they live in [token-rollup.md](token-rollup.md).

## 1. What it reports

For each UTC day and model, `stats_daily` carries three second counters beside the
token total. Both clients draw them: the terminal as a bar plus per-bucket
sparklines, the dashboard as a proportion.

The three are not a partition of the wall clock. They are the sum of the intervals
the fleet was *observed* in each state, and a state with no bucket — `ended`,
`stale`, `error`, `stalled`, `compacting`, `thinking` — contributes nothing to any
of them. The bar shows where the observed time went, not where the day went.

## 2. Only hooks close an interval

An interval is closed when a hook event arrives: the server takes the time since
that session's previous event and adds it to the previous status's bucket.

**The watcher's reports never close one.** It reports every session every couple of
seconds, and those reports are deliberately kept out of the event log (#258) —
without that, the log would be almost entirely watcher scans, and the log is what
the interval is measured against.

The consequence is the point of this section: **a machine covered only by the
watcher accrues tokens and no time at all.** That is not a fleet with nothing to
report. It is a fleet whose durations nobody is in a position to measure. It
happens on a machine where `vigie hooks install` was never run, and for a session
that was already open before the hooks arrived.

## 3. Why the watcher is not made to close intervals

It could report the transitions — it observes them, that is what it exists for.
Doing so means either writing its reports into the event log, which #258 removed
on purpose, or giving the rollup a second source of truth for "when did this
session last change state", which is a second thing to keep correct.

Neither is wrong; both are a decision about the event log rather than a fix to the
Stats tab, and neither was taken here. What was taken here is that **the panel must
not describe its own blindness as an absence of activity** (§ 4).

## 4. The empty state says which silence it is

"No activity yet" is true of a fleet that has just started and of a fleet that will
never report a second, and those want different things from the operator: waiting,
versus installing the hooks. The clients say that time comes from the hooks rather
than leaving the reader to conclude their fleet is idle.

## 5. Rejected

**Deriving durations from `last_seen_at` on the server.** It would give every
machine a timeline, watcher-only ones included. It also invents transitions the
fleet never reported: a session read as `working` for the whole gap between two
scans, whatever happened inside it, and `stats_daily` is never recomputed
(token-rollup.md § 1), so a guess written there is permanent. The rollup is
conservative by construction; this would make it confident instead.
