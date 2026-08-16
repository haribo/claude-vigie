# Session list — Design Specification

**Status:** Accepted.

Source of truth for how the operator shapes the sessions table — **sorting,
filtering, grouping, and visibility** — the user-observable behavior, not the
code. Status semantics live in [session-status.md](session-status.md).

Everything here shapes your **view** and may be persisted. vigie holds no
*handling* state about a session (unread/ack, dismiss, snooze, resolve, pin) —
that belongs in the session itself. vigie is **read-only to the operator**
([ADR-0007](../adr/0007-read-only-to-operator.md)); being alerted to a session that
needs you is an outbound signal (desktop notifications, the `n` jump), not a stored
state.

---

## 1. Visibility — what shows by default

The table is a live operator view, not an archive. By default it hides the noise:

- **Ended sessions are hidden.** A closed session is done; it drops off the list.
- **Optionally, long-idle sessions are hidden** after a configurable duration
  (Settings tab; off by default). A session idle longer than that threshold
  disappears until it resumes.
- **Hiding ended sessions is a Settings preference** (`hide_ended`), not a
  keystroke. It was bound to `a` until #493 removed it: a bare unmodified letter
  that rewrote `tui.toml` with no confirmation and no undo, while the same setting
  sits one tab away under a readable label with its value on screen. It does
  **not** touch `idle_hide_after`, which is a set-and-forget threshold, not a
  per-glance override.

The bottom bar reports how many sessions the current filter is hiding, so the
operator always knows there is more behind it. Which elements earn a permanent row
at all, and where they sit, is specified in
[sessions-chrome.md](sessions-chrome.md) — the summary strip this section used to
describe was deleted in #492.

**One width budget, right side first.** The bottom bar has two halves — the
subscription gauges on the left, the view state (`sort`, `group`, `hidden`) and
the `h help` hint on the right. On a narrow terminal the right half is measured
first and its space reserved; the gauges then fit into what remains.

The right half is the smaller and the more load-bearing: it carries the two
promises above, and `hidden N` is the only thing on screen saying the list is
filtered. The gauges are figures the Stats tab holds in full, so they are what
gives way.

This paragraph described the deleted summary strip until #531 — its counts,
`activity`, `rc` and `out`, none of which the bottom bar has ever carried. The
rule it states is #486's and still holds; only the bar it was about changed
(#492).

---

## 2. Sorting — `s` to cycle, `S` to reverse

`s` cycles the sort key; `S` reverses the current key. Each key has a **natural
direction** (what you almost always want first), and the sort is stable so equal
rows keep their previous order:

| Key         | Natural order (top first)                                          |
| ----------- | ------------------------------------------------------------------ |
| `last seen` | most recently active (default)                                     |
| `tokens`    | most total tokens                                                  |
| `status`    | most active — see § 2.1                                            |
| `name`      | A → Z                                                              |
| `rc`        | remotely controlled first                                          |

### 2.1 The status order

Every status is ranked. A partial list was the defect in #464: four of the nine
fell to an unranked default, which in the TUI sorted them *below* `ended`, and in
the web dashboard produced `undefined` and a comparator that returned `NaN` — a
sort that stopped ordering rather than mis-ordering.

Top first:

| # | Status | Why here |
|---|--------|----------|
| 1 | `stalled` | A hung tool. The exception to "most active": nothing is happening, and that is the point. |
| 2 | `working` | A turn is running. |
| 3 | `thinking` | A turn is running, inside extended thinking. |
| 4 | `compacting` | A turn is running, summarizing its context ([ADR-0008](../adr/0008-compacting-status.md)). |
| 5 | `waiting` | Stopped, and the operator is the blocker. |
| 6 | `idle` | Alive, between turns. |
| 7 | `error` | Live but not producing: a transient API error it will retry through. |
| 8 | `stale` | No fresh signal and no watcher — the state is unknown, not known-inactive. |
| 9 | `ended` | Over. |

**This is not a new order.** Ranks 1–8 are the order the web dashboard already
encoded, and the five the TUI ranked agree with it pairwise; only `compacting` was
missing from both, and it sits with the other two mid-turn sub-states. Preserving
every existing pairwise relation was deliberate — the defect was the omissions, not
the ordering.

**`error` sits low on purpose, and it is worth naming the tension.** It is one of
the three statuses the TUI notifies on (`waiting`, `error`, `stalled`), so it
demands attention; yet the key sorts by *activity*, and an errored session is
producing nothing. Attention is carried by the notification and the colour, not by
the sort. A reader who expects attention-first ordering should read this row as the
answer, not as an oversight.

`tokens`, `status`, and `rc` break ties by most-recently-seen, so within a rank
the freshest session is on top. The active key and direction are shown in the
bottom bar, and the direction is also readable from the arrow in the column
header.

---

## 3. Filtering — `/` to search

`/` opens a filter that narrows the list as you type:

- **Fuzzy match** (default): the typed characters must appear *in order*
  (case-insensitive subsequence) anywhere across a session's name, machine,
  project, branch, and status. `wbp` matches `web-app`.
- **`rc`** is a special filter: typing exactly `rc` isolates the
  remotely-controlled sessions instead of fuzzy-matching.

Filtering composes with visibility and sorting: it narrows what is already
visible, then the result is sorted. The selection resets to the top as the
filtered set changes.

---

## 4. Grouping — `g` to cycle

`g` cycles grouping: **off** → **machine** → **project**. When grouped, rows are
clustered under their group with a per-group subtotal, so a fleet spread across
machines (or a machine running several projects) reads at a glance. Grouping is
applied after sorting, so the order within each group still follows § 2.

---

## Appendix — doc conventions

`docs/design/` = the *what* (user-observable); `docs/adr/` = decisions with
rationale; code = the *how*. Docs never paraphrase code. Adding/modifying design
docs requires explicit consent — propose first.
