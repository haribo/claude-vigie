# Watcher liveness — Design Specification

**Status:** Proposed (#386).

Source of truth for how the fleet knows a machine's watcher is **running**. Version
consistency — whether that watcher is *allowed* to write — lives in
[version-consistency.md](version-consistency.md).

---

## 1. The defect this replaces

The watcher had no heartbeat of its own: liveness was a side effect of session
data. Every report it sent was an `Event: "watch"` carrying a `SessionID`, and the
server recorded `watch_seen:<machine>` only while handling one.

So a machine whose watcher was healthy but currently scanned **zero** sessions —
none open, or all older than `--max-age` — sent nothing at all, and read as having
no watcher: a stale entry in `GET /api/watcher`, a "running on hooks alone" flag in
the Machines tab, and a TUI preflight that refused to start while blaming the
server or the network.

**Liveness must not depend on there being sessions to report.** That coupling is
the bug; a fix that keeps it in a subtler form (an idle beat sent *only when* the
scan is empty) is not a fix.

## 2. A dedicated endpoint

`POST /api/watcher/heartbeat`, carrying the machine and the watcher's build:

```json
{"machine": "minet", "watcher_version": "0.5.0", "watcher_commit": "a1b2c3d"}
```

Rejected alternatives:

- **A session-less report on `/api/report`.** It would mean relaxing the
  `session_id` requirement, weakening a validation that today catches malformed
  reports, and it keeps two unrelated meanings on one route. The conflation of
  "session state" with "I am alive" is what produced this defect; the fix is to
  separate them, not to formalise the overlap.
- **An idle beat only when the scan is empty.** Cheaper, but liveness stays a
  function of session state — a subtler version of the same coupling.

The daemon:

- **always records** `watch_seen`, `watch_seen:<machine>` and
  `watch_version:<machine>` — a heartbeat is an identity claim, and recording it is
  precisely what keeps a machine visible;
- answers **`409`** when the build is drifted (the rule and its rationale are in
  [version-consistency.md](version-consistency.md)), else **`204`**.

A drifted machine therefore stays visible *because* its heartbeat is still
recorded, while its session state stays refused.

## 3. Cadence, decoupled from the scan

The watcher beats **unconditionally**, at most every **5 s** — comfortably inside
the 15 s staleness threshold the TUI and the Machines tab use, and independent of
the 2 s scan interval so a fast scan does not multiply requests.

Session reports keep updating `watch_seen` as they always did. That path is now
redundant for liveness but stays: it costs nothing and keeps a machine running an
older watcher visible.

## 4. The heartbeat drives the drift state

The heartbeat's answer is what tells a watcher whether it may report, replacing
the "retry one session report every 60 s" probe from #384 — which could never work
on the machine this document is about, since a drifted watcher with zero sessions
has no report to probe with.

- `204` → the watcher reports normally; if it was drifted, it announces the
  recovery and resumes.
- `409` → the watcher stays **inert**: it keeps beating (so it stays visible) and
  stops sending session state.

A drifted watcher is therefore visible, silent about session state, and
self-healing — with one mechanism instead of two.

**The startup probe of `GET /api/version` stays.** It covers what the heartbeat
cannot: a watcher newer than its daemon, where `/api/watcher/heartbeat` does not
exist yet and answers `404`. That is a transport failure, not a drift signal, so
without the probe such a watcher would never learn why it is being ignored.

Transient failures (server down, `404` from an older daemon) are **not** drift:
the watcher keeps reporting and keeps beating, and announces the condition only on
transition, so a persistent outage does not fill the journal.

## 5. Testing

- **Server** — a heartbeat records `watch_seen`/`watch_seen:<machine>`/
  `watch_version:<machine>` and answers `204`; a drifted heartbeat answers `409`
  **and still records** them; the route rejects an unauthenticated request.
- **Visibility** — a machine known only from a heartbeat, with no session ever
  reported, appears in `GET /api/watcher` with a fresh timestamp. This is the
  regression test for #386.
- **Watcher** — the heartbeat is sent when no session exists at all; a `409`
  suspends session reports while beats continue; a `204` after a `409` resumes
  them; a transient failure does neither.
