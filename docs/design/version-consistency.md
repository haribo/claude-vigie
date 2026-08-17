# Fleet version consistency — Design Specification

**Status:** Accepted (#384).

Source of truth for the rule that **every vigie component talking to a daemon must
share its build**, and for what happens when one does not. The TUI's startup
sequence lives in [tui-preflight.md](tui-preflight.md); this document owns the
*rule* and its enforcement across the fleet.

---

## 1. The rule

A component may only exchange state with a daemon whose build matches its own:

- **strict equality** of the version string when both sides are a real release;
- when **either side is `dev`**, the **commit** is compared instead — a
  `"dev" == "dev"` string match across two different builds is a false pass.

This is one rule with **one implementation** (`internal/version`), used by every
enforcement point. Two copies of a consistency rule drift, which is the defect
class this document exists to remove.

## 2. Why the watcher, not just the TUI

Before #384, enforcement existed only in the TUI preflight. The watcher merely
*declared* its build in the heartbeat (#356) and never verified anything, so a
version was validated only **transitively** — when a TUI happened to start on
that same machine. The consequences:

- a machine running a watcher where nobody opens a TUI was **never** checked and
  could drift indefinitely while writing session state into the server;
- the consumer (TUI) policed the producer (watcher), while the risk lives with
  the producer: it runs continuously and feeds the database;
- by [ADR-0009](../adr/0009-watcher-managed-hooks.md) the watcher owns the hooks
  lifecycle, so a drifted watcher installs drifted hooks — the drift propagates.

## 3. Enforcement is server-side

The daemon is the authority on its own build, and the watcher's build already
travels in **every** watch report (`watcher_version`/`watcher_commit`, #356). So
the check costs no extra round-trip and belongs where it cannot be bypassed:
enforcement that lives only in the client can be skipped by exactly the outdated
client it is meant to stop.

On a report with `Event == "watch"`, the daemon compares the report's build to
its own:

- **match** → the normal path, unchanged;
- **mismatch, or no build declared at all** (a watcher older than #356 cannot
  identify itself, and is by definition drifted) → the daemon
  - **still records** `watch_seen:<machine>` and `watch_version:<machine>`, so
    the machine and its drifted build stay visible fleet-wide — the Machines tab
    already renders that version (#356), which is what tells the operator *which*
    machine to upgrade and *to what*;
  - **does not** upsert the session, append an event, roll up tokens, sample, or
    publish over SSE — no drifted state ever reaches the database;
  - answers **`409 Conflict`** with a message naming both builds.

### Visibility must not be derived from sessions

Because a drifted watcher writes **no sessions**, `GET /api/watcher` cannot build
its machine list from sessions alone — that would hide exactly the machine the
operator needs to see. It lists the union of machines that have sessions and
machines the server has heard a watch report from (a recorded
`watch_seen:<machine>`). Refusing a machine's data and erasing the machine from
the fleet view are not the same thing, and only the first is intended.

### Known limit: subscription usage is not gated

`POST /api/usage` carries no watcher build and reports machine-level subscription
usage rather than session state, so it stays outside this gate. The drift that
motivated #384 — a drifted watcher writing *session* state — is closed; usage
from a drifted machine is not. Recorded here as a limit rather than left as an
unstated gap.

`409` is the pragmatic choice: the request conflicts with the state of the target
(its build). `426 Upgrade Required` is semantically about switching HTTP protocol
and mandates an `Upgrade` response header, which does not apply here.

### Hook reports are deliberately out of scope

`vigie report`, invoked by the hooks, runs **inside the operator's Claude Code
session**. Blocking or erroring there risks disturbing real work for a monitoring
concern, which the observe-only stance ([ADR-0005](../adr/0005-observe-only.md))
forbids. Hook reports carry no `watcher_version` and stay ungated. This costs
little: by ADR-0009 the watcher owns the hooks and *is the same binary*, so gating
the watcher already governs the build those hooks invoke. An explicit decision,
not an omission.

## 4. The watcher: loud, inert, self-healing

A drifted watcher **must not exit.** `docs/deployment.md` ships the unit with
`Restart=on-failure` / `RestartSec=5`, so `exit 1` would crash-loop every five
seconds and cost the machine all observability exactly when the operator needs
it; `exit 0` would die silently, and since #371 the TUI would then report
"watcher not running", pointing at the wrong remediation.

Instead, on a `409` the watcher enters a **drifted** state:

- it stops scanning and reporting session state;
- it logs **one** clear line naming both builds and the remediation — announced on
  transition, not repeated every scan;
- it keeps sending its **liveness heartbeat**
  ([watcher-liveness.md](watcher-liveness.md)), which both keeps the drifted
  machine visible and carries the answer that ends the drift: a `204` means the
  builds have realigned, and the watcher resumes on its own.

> **Amended by #386.** This originally re-probed with a single *session report*
> every 60 s. That could never work on a machine with no sessions — there was no
> report to probe with, and such a machine was invisible anyway. The heartbeat
> replaces it: one mechanism, and it works with zero sessions.

**Startup fast-path.** At startup the watcher fetches `GET /api/version` once and,
on mismatch, reports the drift immediately with its remediation rather than
waiting for the first scan to be refused. If the server is unreachable or errors,
**unknown is not drifted**: the watcher starts normally and lets the server
arbitrate, the same "don't cry wolf" principle the stale-watcher banner already
applies.

## 5. Testing

- **Server** — a watch report from a drifted build returns `409` and leaves the
  session unwritten, while `watch_seen`/`watch_version` are still recorded; a
  matching build behaves exactly as before; a hook report (no declared build) is
  unaffected; a watch report with an empty build is treated as drifted.
- **Watcher** — a `409` puts the loop in the drifted state and stops session
  posts; the re-probe fires on its slow cadence rather than every scan; an
  accepted probe resumes normal reporting.
- **Shared rule** — the release/`dev`-commit matrix is tested once, against the
  single implementation in `internal/version`.
