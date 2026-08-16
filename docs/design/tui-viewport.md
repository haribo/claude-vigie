# TUI vertical viewport — Design Specification

**Status:** Accepted (#378).

Source of truth for how the sessions table fits the terminal **height**. The TUI
already refuses to scroll sideways — every layout guard fits the **width**
([session-list.md](session-list.md), width-sweep tests). It says nothing about
height: `WindowSizeMsg.Height` is discarded, so a fleet with more sessions than
the terminal has rows spills off the bottom and the alt-screen clips the excess —
the core view silently loses sessions. This spec adds the vertical dual of the
width discipline: the table **scrolls within a bounded viewport**, never past the
screen edge.

---

## 1. Principle

The sessions view is a stack of **fixed chrome** framing one **scrollable row
band**:

```
tab bar                     ┐
[⚠ watcher-stale]           │ fixed (top)
summary strip               │
rule                        │
[filter line]               │
[overflow banner]           ┘
column header               ← pinned
 …session rows…             ← the only scrollable band
[position indicator]        ← pinned (only when the band overflows)
rule                        ┐
usage / platform strip      │ fixed (bottom)
rule                        │
footer                      ┘
```

Only the row band scrolls. Everything else stays put, so the operator never
loses the header, the counts, or the key hints to scrolling.

Which elements earn a permanent row at all is specified separately, in
[sessions-chrome.md](sessions-chrome.md): this document budgets the rows, that
one decides who gets them.

**Chrome costs one row, whatever the width.** The key-hint footer is fitted to
the terminal width rather than wrapped: hints are dropped whole, least essential
first, and the cut is marked with an ellipsis. `q quit` is never dropped — the
way out stays on screen at every width. Wrapping is measured correctly by the row
budget below, so nothing is drawn out of place, but the row is genuinely spent:
the Sessions footer needs 134 columns, so every narrower terminal paid a second
row for it on every frame. Rows belong to the session table, and a standing
reminder of `q quit` is not worth two of them on a 24-row terminal (#487).

## 2. Row budget

`View` learns the terminal height from `WindowSizeMsg.Height` (new `model.height`,
the vertical dual of `model.width`). The row budget is computed by **measuring the
rendered chrome**, not by hard-coding line counts — the chrome is variable
(the watcher-stale line, the filter line, the multi-line overflow banner and a
wrapped usage strip all come and go):

```
rowBudget = height − lines(everything rendered except the row band)
```

Measuring the actual rendered strings keeps the budget correct as chrome appears
and wraps, with no second source of truth to drift. If `height` is 0 (not yet
received) or `rowBudget < 1`, the table renders **unbounded** (today's behavior) —
degrade to the old overflow rather than render nothing.

## 3. Scrolling — continuous, cursor-driven, sticky

Continuous scroll like `htop`/`k9s`, **not** paging:

- The cursor moves row by row (`↑`/`↓`, `j`/`k`); the band scrolls only when the
  cursor would leave it. The offset is **sticky** — it does not recentre on every
  keystroke (jarring); it shifts by the minimum needed to keep the cursor visible.
- A **scroll-off margin** of 2 rows: the cursor stops 2 rows from the top/bottom
  edge while more rows exist that way, so there is always look-ahead context.
- The offset is stored (`model.rowOffset`) but is **derived state**: a pure helper
  re-clamps it against the current `rowBudget` and cursor on every render, so a
  resize, a filter change, or a sort can never strand the cursor off-screen. No
  handler needs to know the budget.

## 4. Grouping and the pinned header

Windowing operates in **rendered-line space**, not session-index space, so group
headers (which interleave rows under `group by`) are counted correctly with no
special arithmetic:

1. render the full table (header + any group headers + rows) to lines;
2. keep line 0 (the column header) pinned;
3. window the remaining lines so the selected row's line stays within the budget,
   honoring the scroll-off margin;
4. when `group by` is active and the topmost visible line is mid-group, re-emit
   that group's header as a sticky first band line, so a scrolled view always
   names the group it is showing.

## 5. Position indicator

When the band overflows, a dim right-aligned indicator sits on the pinned line
below the band: `rows 12–28 / 63`. It is shown **only** when scrolling is in
effect — a list that fits carries no indicator and loses no row to it.

## 6. Scope

- **In:** the sessions table (the actual defect) and its detail view (bounded to
  height; a long detail scrolls the same way).
- **Deferred, tracked, not silently dropped:** the Stats, Machines, and Settings
  tabs can also exceed a very short terminal. They are short in practice; bounding
  them is a follow-up, not this change. Until then they keep today's behavior.
  This spec is amended before that work, not the code first.

## 7. Testing

Extend the width-sweep harness ([scaling_test.go](../../internal/tui/scaling_test.go))
with a **height sweep**: for a populated fleet, render across a range of terminal
heights and assert (a) the output never exceeds `height` lines, (b) the selected
row is always visible, (c) the pinned header and summary are always present, and
(d) the position indicator appears exactly when the band overflows. A pure
`window(lines, selected, budget, offset)` helper carries its own unit tests
(top edge, bottom edge, scroll-off, resize re-clamp, list-shorter-than-budget).
