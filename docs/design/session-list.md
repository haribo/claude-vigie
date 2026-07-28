# Session list — Design Specification

**Status:** Accepted.

Source of truth for how the operator shapes the sessions table — **sorting,
filtering, grouping, and visibility** — the user-observable behavior, not the
code. Status semantics live in [session-status.md](session-status.md).

---

## 1. Visibility — what shows by default

The table is a live operator view, not an archive. By default it hides the noise:

- **Ended sessions are hidden.** A closed session is done; it drops off the list.
- **Optionally, long-idle sessions are hidden** after a configurable duration
  (Settings tab; off by default). A session idle longer than that threshold
  disappears until it resumes.
- **`a` shows everything** — a momentary "show all" that reveals the hidden
  ended/idle rows without changing the saved preferences.

The summary strip reports how many sessions the default view is hiding, so the
operator always knows there is more behind `a`.

---

## 2. Sorting — `s` to cycle, `S` to reverse

`s` cycles the sort key; `S` reverses the current key. Each key has a **natural
direction** (what you almost always want first), and the sort is stable so equal
rows keep their previous order:

| Key         | Natural order (top first)                                   |
| ----------- | ----------------------------------------------------------- |
| `last seen` | most recently active (default)                              |
| `tokens`    | most total tokens                                           |
| `status`    | most active — `working` › `waiting` › `idle` › `ended`      |
| `name`      | A → Z                                                        |
| `rc`        | remotely controlled first                                   |

`tokens`, `status`, and `rc` break ties by most-recently-seen, so within a rank
the freshest session is on top. The active key and direction are shown in the
summary strip.

---

## 3. Filtering — `/` to search

`/` opens a filter that narrows the list as you type:

- **Fuzzy match** (default): the typed characters must appear *in order*
  (case-insensitive subsequence) anywhere across a session's name, machine,
  project, branch, and status. `plnt` matches `plain-note`.
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

## Appendix — doc conventions (from tribnest)

`docs/design/` = the *what* (user-observable); `docs/adr/` = decisions with
rationale; code = the *how*. Docs never paraphrase code. Adding/modifying design
docs requires explicit consent — propose first.
