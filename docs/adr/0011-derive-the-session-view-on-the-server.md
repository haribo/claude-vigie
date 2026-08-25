# ADR-0011: Derive the session view on the server; verify the rest with shared fixtures

## Status

Accepted.

## Context

vigie has two clients for the same board — the TUI (Go) and the web dashboard
(JavaScript) — and a third, partial one, the GNOME indicator. `/api/sessions`
returns the stored row, so every client derives the view from it *itself*.
`internal/web/static/lib.js` now exports 47 symbols, most of them hand
transcriptions of functions in `internal/tui`: `contextWindow`/`contextPct`,
`rank`, `sessionName`/`shortId`, `apiErrorLabel`, `detailText`, `modeLabel`,
`fuzzyMatch`/`matchesFilter`, `groupKeyOf`, `hiddenByIdle`, `humanTokens`,
`relAge`, the column migrations. `gnome-extension/lib.js` holds a fourth copy of
the attention rules.

`architecture.md` binds the clients on **content** — same statuses, same
attention set, same verdicts on what earns a row — and calls a divergence debt.
The mechanism that is supposed to hold that line is a hand copy.

**The cost is measured, not feared.** Two forms:

- *Every feature is built twice.* v0.6.0 shipped the filter (#545), grouping
  (#546), idle-hiding (#547) and four columns (#550) into the dashboard —
  behaviors the TUI already had.
- *The copies drift, silently.* #421, #422, #423, #464 and #466 are all one rule
  updated in one copy and not the others. The most recent instance is not
  historical: #599 and #600 built a watcher alarm that names the machines whose
  watcher stopped, and the dashboard has none of it — it fetches `/api/watcher`
  and reads only `versions`, computing no freshness verdict at all. An operator
  watching from a phone is not told that a machine went silent.

Three mitigations already exist, unevenly applied:

1. **Hand transcription** — the default, and what produced the bug class above.
2. **Regex scrapes** — Go tests that read the JavaScript source and pull constant
   arrays out of it (`jsArrayFromFile`). They work, and their existence is the
   symptom: the invariant has no structural support, so it is re-checked by
   string matching.
3. **Shared behavioral fixtures** — one list of cases in `test/fixtures/`, read by
   the Go tests *and* the JavaScript tests, each implementation proved against it.
   Built for exactly three rules: the subsequence filter, the columns, the API
   error labels. This is the mature answer and it was left unfinished.

## Decision

**Two halves, two answers, in that order.**

**1. What derives from a session alone is derived once, on the server.**
`SessionView` carries the computed fields — context fill, sort rank, whether the
session is in an attention state, its display name, its error label — and clients
render what they are given. `internal/status` already exists for this reason; the
derivation joins it rather than being re-typed per client. The GNOME indicator is
fixed by the same move, having no test suite of its own to catch a drift.

**2. What depends on the operator stays client-side, and is proved against a
shared fixture.** The filter query, the grouping mode, the idle threshold, the
column layout are functions of what the operator typed or chose; moving them to
the server would mean shipping every keystroke to it. They stay duplicated — and
every one of them gets a case list in `test/fixtures/`, read by both suites, on
the pattern already used for three of them.

**A third category, found while applying the second (#617): what depends on the
passing of time stays client-side.** A session's context fill does not change on
its own; a watcher's freshness does — it decays with nothing happening at all. A
verdict computed by the daemon is frozen at the moment it is sent, and the
dashboard refetches that endpoint once a minute against a threshold of fifteen
seconds, so a watcher that died would read as live for up to a minute. Computed
from the timestamp by the client, it decays correctly between requests and needs
no round trip. Time belongs to whoever is displaying, and a derived value that is
a function of *now* has to be derived there.

Such a value takes the second half's treatment, not the first: it stays where it
is and earns a shared case list.

**The scrapes are deleted by whichever half covers them.** A constant array that
survives as client-side data becomes a generated fixture; one that moves to the
server stops existing twice. `jsArrayFromFile` having no callers left is the
measurable end state of this ADR.

**Order matters and is part of the decision.** The server half comes first,
because it shrinks the set the fixtures must cover; writing fixtures first means
writing some that the next step deletes.

## Rationale

Neither half is sufficient alone, which is why this ADR is not a choice between
them:

- **Server derivation alone** leaves the operator-dependent half hand-copied and
  mostly unguarded — the half that carries the filter and the grouping, where
  #545–#547 already went twice.
- **Fixtures alone** make every duplicated rule verified, which kills the drift
  bug class, and leave the double build cost entirely intact. Cheap, and it stops
  short of the thing that actually costs.

## Alternatives rejected

- **Generate `lib.js` from Go.** Covers everything, including the pure
  operator-dependent functions, and would delete the duplication rather than
  verify it. It requires transpiling a subset of Go to JavaScript, or maintaining
  the generator that does — a project of its own, well beyond the surface it
  serves. A cheaper reading (generate only the constant tables) buys no more than
  the scrapes already do.
- **Accept the duplication and drop the guards.** Named only to reject it: the
  bug class is documented five times over.

## Consequences

- `SessionView` grows. The API surface widens deliberately, and the daemon
  becomes the place a new derived field is added — one implementation, one test.
- **The dashboard can only show what the daemon computed.** Version skew would
  matter, except the fleet already refuses a client whose build does not match the
  daemon (#356, #384), so both advance together by construction.
- A model's context window moves server-side, so teaching the fleet a new model
  becomes a daemon upgrade rather than a client one. Under the same drift gate,
  that is the same upgrade.
- The migration runs per family — context, then status/attention, then naming and
  labels, then the operator-dependent fixtures — so no single change carries the
  whole surface.
- Any new client-side derivation must first be reconciled with this ADR: it either
  belongs on the server, or it is operator-dependent or time-dependent and arrives
  with its fixture.

## References

- [architecture.md](../architecture.md) — what "mirror" binds between clients
- [ADR-0003](0003-split-client-and-daemon-binaries.md) — the client stays minimal;
  deriving on the server moves work away from it, never toward it
- Issue #603, and the drift instances #421, #422, #423, #464, #466
