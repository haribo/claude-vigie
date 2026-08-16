# Sessions chrome and the state pill — Design Specification

**Status:** Proposed (#491).

Source of truth for what the Sessions tab keeps permanently on screen around the
table, and for how the operator learns that something between them and the truth
is broken. It sits above [tui-viewport.md](tui-viewport.md), which specifies how
the remaining rows are budgeted, and beside
[session-list.md](session-list.md), which specifies the table itself. The table
is unchanged by this document: columns, ordering, grouping, filtering and the
column picker all stay exactly as specified there.

---

## 1. The problem

The Sessions tab spends **10 of the terminal's rows on chrome** and 5 full-width
rules, on a screen whose only job is to show sessions. On a 24-row terminal that
leaves 14 rows for the table.

The count is the smaller half of the problem. The chrome mixes four unrelated
scopes at the same visual rank — the **fleet** (status counts), the **account**
(usage quotas), the **vendor** (Claude platform health) and the **view** (sort,
group, hidden) — with nothing to say which of them asks for an action.

And **six separate indicators answer variants of one question**, each with its
own glyph, wording and location:

| Indicator | Where | What it invalidates |
| --- | --- | --- |
| `● ◍ ○` (`connGlyph`) | end of the summary strip | everything on screen |
| `⚠ no watcher reporting` | line under the tabs | every status |
| `⚠ could not refresh …` | line above the table | the table |
| `⚠` (`staleMark`) | end of the bottom strip | the figure beside it |
| `⟳` (`syncGlyph`) | bottom strip | the two usage gauges |
| `platform ● operational` | bottom strip | nothing — it *explains* |

The first five ask *is what I am looking at true?* at five granularities, and the
consequence for the operator is the same every time. The sixth is not about vigie
at all.

---

## 2. What earns a permanent row

A row of chrome is permanent only if it is **true of every glance**. Everything
else lives one keystroke away.

Three tests, all of which a permanent element must pass:

1. **It is not already on screen.** `● working 3 · ● idle 1 · ● stalled 1` is the
   exact aggregate of the STATUS column; the active sort is already readable from
   the arrow in the column header; `hidden 25` and `● ended 25` say the same
   thing twice on the same line.
2. **Its absence is not itself the signal.** A healthy state needs no row: it is
   the default, and a row that reads `operational` 99 % of the time trains the eye
   to skip the place where the exception will appear.
3. **The operator can act on it, or is misled without it.** `hidden N` survives on
   this test alone: `a` and `hide idle after` filter silently, and without it the
   screen claims five sessions while the fleet has thirty.

Target: **6 chrome rows, 3 rules, two scopes** — navigation on top, figures at
the bottom, nothing but sessions between them.

```
tab labels + [i]●            ┐ navigation
underline                    ┘
column header                ← pinned
rule                         ← the only rule between two different kinds of content
 …session rows…              ← the scrollable band
rule                         ← closes the table
usage gauges · view state · hidden N · h help
```

Five identical full-width rules establish no hierarchy — they cut the screen into
slices of equal importance. Three remain: the one closing the column header, the
one closing the table, and the tab underline that already exists.

---

## 3. The state pill

**One indicator, top right, preceded by its `[i]` keycap, as the last character
of the tab line.** That corner is where the eye checks for state, and where a
pulse is most detectable in peripheral vision.

**The corner never changes width.** No text ever appears beside the pill,
whatever happens, so the table below never jumps.

Three levels, encoded in **shape as well as colour**, so a monochrome terminal
and a colourblind operator read the same thing. The vocabulary is `connGlyph`'s,
already shipped:

| Glyph | Colour | Meaning |
| --- | --- | --- |
| `●` | green | every layer healthy |
| `◍` | amber | a layer is degraded, but **nothing displayed is false** |
| `○` | red | **the screen may be lying** |

### The sorting criterion is not severity

It is *is something on screen false?*

A Claude platform outage is severe and never turns the pill red: vigie is
correctly reporting sessions that are correctly erroring. It is the cold outside,
not a dead battery in the thermometer. The same holds for an aged usage snapshot
and a client/daemon version drift — degraded, honestly displayed, amber.

Red is reserved for the cases where the operator would draw a wrong conclusion
from what is drawn: the link is down, no watcher is reporting, the sessions are
frozen.

---

## 4. Unknown is not failure

When the vigie server is unreachable the TUI loses the **ability to observe**,
not merely the data. Showing the last known watcher or platform value as if it
were current is exactly the lie that #449 and #456 exist to prevent.

The split is **by who observes what**:

| Class | Glyph | What falls in it |
| --- | --- | --- |
| observed failure | red | what the TUI establishes itself: the link, the sessions poll that just failed |
| unknown | grey `◌` | what it only knows *through* the server: watcher, Claude platform, daemon version |
| degraded but known | amber | what it holds locally with a timestamp: the usage snapshot |

The grey `◌` is not a new glyph: it is already the `stale` session status,
adopted in #284/#285 on exactly this reasoning — *no news ≠ dead*, and an unknown
beats a false `ended` ([session-status.md](session-status.md) § 1).

Greying the usage snapshot would hide the one thing still true about it: its age
is known offline.

**Counter-check.** A Claude outage with the link intact leaves every row *known*
and the pill amber. Nothing is grey — vigie observes a broken platform perfectly
well.

---

## 5. The state modal, behind `i`

`i` opens it from any tab and returns the operator exactly where they were. It
overlays rather than pushes, so the vertical budget never depends on it, and it
captures keys while open.

```
╭─ State ─────────────────────────────────────────────────╮
│                                                         │
│   ○ vigie server      offline · last poll failed 2m ago │
│   ○ sessions          frozen · showing last known       │
│   ◌ watcher           unknown · needs the server        │
│   ◌ claude platform   unknown · needs the server        │
│   ◌ client / daemon   unknown · needs the server        │
│   ◍ usage snapshot    38m old · cannot refresh          │
│                                                         │
╰─ i or esc to close ─────────────────────────────────────╯
```

**Rows use the pill's vocabulary**, the same three shapes plus grey `◌`, so the
pill is a summary of the modal and not a second language. The worst row is the
pill: any red makes it red, else any amber makes it amber. A grey row is not a
level — it is the absence of one, and it never colours the pill by itself; what
turns the pill red in that case is the observed failure of the link above it.

**Row order encodes the dependency**: what the TUI observes alone, then what
transits through the server, then what is local. A reader can see why three rows
are grey by reading the two above them.

**Two renames, because the current labels mislead.** `platform` becomes `claude
platform` — the bare label reads as though it were about vigie's own health, and
it is Claude's Statuspage, polled by the server and fanned out to clients
([ADR-0005](../adr/0005-observe-only.md)). The connection row is named `vigie
server`.

**A seventh line that has no useful visibility today**: the client/daemon version
drift, buried in Settings although it fails confusingly across a fleet (#341).
That is what makes this more than a relocation of glyphs — the whole chain
between the operator and the truth, in one place.

The **Machines tab** was the alternative host. The modal wins because it opens
from any tab without losing the operator's place, and because half of these are
not per-machine facts: the link, the sessions refresh, the platform and the
versions are global.

### What this does not destroy

Per-source granularity exists because a panel that fails silently is a lie the
operator cannot see through (#449, #456). It does not disappear and it does not
move: the modal is **added** to it, not substituted for it.

An earlier draft had the failure banners (`staleNote`, `staleReason`, the watcher
warning) removed in favour of the modal, on the grounds that they stopped costing
a standing row. That premise is wrong — those lines are conditional, so they cost
nothing while everything is healthy, and they appear exactly when the operator
needs them, in the panel they are about. Deleting them would trade a warning at
the point of use for one a keystroke away, and buy nothing. They stay; the modal
answers a different question, which is *why*, for the whole chain at once.

---

## 5bis. The shortcuts modal, behind `h`

Eleven key hints sat permanently at the bottom of the Sessions tab. They need 134
columns; below that the trailing ones are dropped and the cut marked with an
ellipsis (#487) — a row permanently occupied *and* incomplete. Eleven reminders
are never needed at once.

**One hint, `h  help`**, at the end of the bottom bar. `h` opens a modal listing
every shortcut of the active tab on two columns; `h` or `esc` closes it. Like the
detail panel it replaces the body rather than pushing it, so the vertical budget
never depends on it, and it captures keys while open.

**`h` is the help key on every tab.** It was already bound in Settings as a vim
alias for left/decrement, alongside `l`. Those two aliases are dropped: they were
advertised nowhere — the Settings hints have always read `space/←→ change` — and
one help key everywhere is worth more than an alias nobody was told about. `j`
and `k` keep their row movement, and the arrows keep everything.

**The `a` binding is removed.** It is not a view shortcut: it flips `hideEnded`
and saves, so a bare unmodified letter silently rewrites `tui.toml`, with no
confirmation and no undo, while the same setting sits one tab away under a
readable label with its value on screen. The counter-argument would hold for a
filter flipped ten times an hour; hiding ended sessions is set once.

Tabs other than Sessions have no bottom bar; they keep a single-hint row so the
help key is discoverable there too.

---

## 6. The pulse

**A degraded pill breathes — amber and red both. Green never animates.** A gentle
modulation of the glyph's colour toward a second tone of the same hue, the glyph
always present.

A static one-character glyph in a corner is not read: peripheral vision detects
motion and luminance, not hue or shape. Since no text ever appears beside the
pill, **the pulse is the whole alert**.

Two constraints follow, both load-bearing:

- **A slow cycle, ~2 s.** The call marker's half-period is 500 ms
  ([ADR-0010](../adr/0010-session-raised-operator-call.md)), and the cadence is
  what separates the two messages: *come now* against *still broken*. There is a
  cost argument too — the blink tick is scheduled only while something animates,
  so a long-lived degraded state pins the TUI to a redraw loop. At 2 s that is
  negligible; at 500 ms over hours, on a tool meant to stay open all day, it is
  not.
- **No preference mutes it.** With no text beside the pill, muting the pulse
  makes a degraded state completely silent. This matches the removal of the
  `blink` preference for the same reason: an accelerator the operator cannot see
  is worse than no accelerator (#490, ADR-0010). Should that removal ever be
  reversed, the state pulse still needs a switch **distinct** from the call
  marker's — sharing one is the outcome to avoid in every case.

### It does not collide with the call marker

Three separations, and the design relies on all three: the call marker
*substitutes* the glyph with a blank on its off half-cycle where the pulse only
modulates a colour; the cadences differ by a factor of four; and the call lives in
a table row while the state lives in the corner — position separating them more
reliably than any animation could.

### Colour

Two rules, because this is where it gets botched:

- **The modulation stays within the same hue.** An amber drifting toward orange,
  or a red toward pink, moves the meaning: colour already carries severity.
- **Each theme needs its own pair.** The TUI does not know the terminal's
  background colour, so the second tone cannot be computed by blending toward the
  ground. It is chosen by hand, light and dark, and checked by eye in a real
  terminal — how a pulse is perceived cannot be judged from a mock.

The pair that ships:

| | light | dark |
| --- | --- | --- |
| amber | `#b45309` → `#d08a4a` | `#fbbf24` → `#c08f2a` |
| red | `#dc2626` → `#ec7070` | `#f87171` → `#bd5555` |

Both move *toward the ground*, which is why the direction flips between themes:
the tone lightens on a light terminal and darkens on a dark one. A test measures
the hue distance of each pair and fails past 20°, so the "same hue" rule is
checked rather than merely stated. How a pulse is actually perceived still cannot
be judged from a test — expect to adjust these by eye.

A ~2 s cycle is 0.5 Hz, well under WCAG 2.3.1's three-flashes-per-second ceiling.
Nothing to arbitrate, but it is the reason the cadence may be slowed and never
sped up.

---

## 7. Rejected alternatives

Recorded because an alternative rejected in conversation and never written down
is an alternative that gets proposed again.

| Rejected | Why |
| --- | --- |
| **A consolidated variant** keeping every element and removing only the packaging | 7 rows, nothing lost — and no decision taken. A tidy-up, not a hierarchy. The scopes stay mixed at the same rank. |
| **An attention band** listing `call`/`waiting`/`stalled`/`error` above the table | The calling sessions would appear twice, and a band that appears and disappears makes the table jump. |
| **A permanent sentence beside the pill** | The corner must never change width, and the modal is the single home for anything about state. |
| **A transient reveal** on the transition | Same, plus it is missed by an operator who was not looking at that moment — which is the whole population the pulse exists for. |
| **The Machines tab as the modal's host** | Half of these facts are global, and it costs the operator their place in the table. |

---

## 8. Scope

This document is the design. It is implemented by #492 (the chrome), #493 (the
shortcuts modal), #494 (the state pill and its modal) and #495 (the pulse), in
that order.
