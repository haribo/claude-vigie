# Watcher liveness — Design Specification

**Status:** Accepted (#386).

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
{"machine": "orion", "watcher_version": "0.5.0", "watcher_commit": "a1b2c3d"}
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

## 5. Reading the heartbeat back

Sections 1–4 specify how liveness is *claimed*. This one specifies how a client
turns the claim into a verdict — the half that was left unwritten, and that two
places in the TUI answered differently as a result (#600).

**One rule, one implementation.** A timestamp is read once, by one function, and
every indicator is a caller of it. Two implementations of "is this watcher still
reporting" is what #284 fixed once already; having two again is how it comes back.

The rule takes the recorded timestamp and the current time, and yields one of
three verdicts:

| Recorded timestamp | Verdict | Why |
| --- | --- | --- |
| within 15 s of now | **reporting** | the watcher beat recently; the statuses on screen are being refreshed |
| absent, or older than 15 s | **silent** | nothing is refreshing the statuses; they may be frozen |
| present but unparseable | **unreadable** | there is an answer and it cannot be read |

**Unreadable counts as an alarm, not as silence and not as health.** The
indicator does not answer *"is the watcher process alive"* — it answers *"can the
operator trust what is on screen"*. When the timestamp cannot be read, the answer
to that question is no, for the same reason and with the same consequence as a
watcher that stopped beating. The screen may be lying, which is what the TUI's red
level means (`level` in the state modal).

Rejected: **treating it as healthy.** That was the pre-#600 behavior of the state
pill, whose comment read *"don't cry wolf"*. A monitoring tool that cannot tell,
and answers "all good", has chosen silence over doubt — and the whole purpose of
this indicator is to say when the board stops being trustworthy.

Rejected: **treating it as unknown (grey).** Honest in wording, but grey never
colors the state pill by design — it marks the *absence* of a channel, not a
channel answering unintelligibly — so the operator would see nothing without
opening the modal. That is the "healthy" outcome wearing a different label.

**The cause travels with the alarm.** Unreadable and silent are both alarms, but
they send the operator to different places: one is a fault in what vigie recorded,
the other is a watcher that stopped. Each indicator says which — the state pill in
its detail line, the Machines tab in its cell — so raising the alarm never costs
the operator a search on the wrong machine.

## 6. The fleet verdict

§ 5 reads one machine's heartbeat. This section says what the whole-fleet
indicator — the TUI's state pill, and the modal behind `i` that names the silent
machines — makes of *several* (#599). The Sessions tab carried a banner too until
#650, which said `no watcher reporting` however many were reporting and never
named one.

**It is not the freshest machine.** The daemon keeps a global `watch_seen`
alongside the per-machine keys, overwritten by every heartbeat from any machine,
so it holds the most recent beat *anywhere*. Deriving the fleet verdict from it
means one live watcher hides every dead one: on three machines, `orion` going
silent leaves the pill green for as long as `box` or `nova` keep beating, while
`orion`'s sessions sit frozen with nothing on the main screen saying so. The
indicator gets *less* trustworthy as the fleet grows, which inverts its purpose.

**The verdict is over every known machine.** `GET /api/watcher` already returns
the per-machine map, and the TUI already holds it — the Machines tab draws from
it. The pill reads the same map rather than a summary of it.

### A machine that never beat is not a fault

A machine reporting through hooks alone, with no watcher service, is a
deployment choice. The daemon distinguishes it without ambiguity: an empty
timestamp means no heartbeat was ever recorded, a non-empty one means a watcher
beat at least once. Nothing prunes those keys, so the distinction survives
restarts and session retention.

So the fleet alarm covers machines that **beat and then stopped** — silent or
unreadable per § 5 — and not machines that never beat. Otherwise a fleet with one
deliberately hooks-only machine is red permanently, and an indicator that is
always red is one the operator stops reading; the day a real watcher dies, it says
nothing new.

**With one exception: no live watcher anywhere is an alarm.** Applied literally,
the rule above turns the pill green on a fleet where *no* machine has a watcher —
the first-run case, where `vigie watch` was never started and nothing is
refreshing anything. That is the one situation the indicator exists for, so it
stays an alarm even though no individual machine qualifies as having stopped.

### The alarm names the machines

`watcher · 1 of 3 not reporting (orion)` rather than a fleet-wide yes/no. The
operator learns which host to go to without opening another tab, and the count
says how much of the board is affected. The Machines tab keeps showing every
machine's own verdict, hooks-only ones included — that view answers "what is the
state of each", not "can I trust this screen".

## 7. Testing

- **Server** — a heartbeat records `watch_seen`/`watch_seen:<machine>`/
  `watch_version:<machine>` and answers `204`; a drifted heartbeat answers `409`
  **and still records** them; the route rejects an unauthenticated request.
- **Visibility** — a machine known only from a heartbeat, with no session ever
  reported, appears in `GET /api/watcher` with a fresh timestamp. This is the
  regression test for #386.
- **Watcher** — the heartbeat is sent when no session exists at all; a `409`
  suspends session reports while beats continue; a `204` after a `409` resumes
  them; a transient failure does neither. These are asserted over the loop itself,
  not over its parts (`run_replay_test.go`, #602).
- **Reading it back** — the three verdicts of § 5, and the fact that every
  indicator derives from the same function: a test that reads the rule once must
  be enough to know what each screen will show (#600).
- **The fleet verdict** — a fleet where one machine of three has gone silent
  raises the alarm and names it; a hooks-only machine beside live ones does not;
  a fleet with no live watcher at all does. This is the regression test for #599.
- **Both clients, one case list** — § 5 and § 6 are implemented twice on purpose
  (ADR-0011's third category: the verdict is a function of *now*, so it decays
  where it is displayed). `test/fixtures/watcher-cases.json` is that list, read by
  `internal/tui` and by `test/js`, alarm text included. A test also holds the
  property the duplication buys: the alarm appears as the clock advances, with no
  new answer from the daemon (#623).
