# Session status — Design Specification

**Status:** Accepted.

Source of truth for what a session's **status** means in vigie and what
makes it change — the user-observable behavior, not the code. Like everything in
vigie, status is **detected**, never operator-set
([ADR-0005](../adr/0005-observe-only.md)).

---

## 1. The nine statuses

Every session shows exactly one status. What each tells the operator:

| Status     | Meaning                                                                 |
| ---------- | ---------------------------------------------------------------------- |
| `working`  | Claude is actively producing — a turn is running.                       |
| `thinking` | Claude is reasoning inside a turn — extended thinking, before it outputs text or a tool call. A sub-state of an active turn. |
| `compacting` | Claude is **summarizing its context** to free space — a ~90–170 s silent sub-state of an active turn. Opened by the `PreCompact` hook, closed by the transcript's `compact_boundary` ([ADR-0008](../adr/0008-compacting-status.md), #342). |
| `waiting`  | Claude has stopped and is **waiting on the human** (a prompt or permission). |
| `stalled`  | A turn is **parked on a hung tool** — a `tool_use` never got its `tool_result` and the session has gone quiet. Distinct from idle: the turn is unfinished, not between turns. |
| `idle`     | The session is open and alive but between turns — nobody is acting.     |
| `error`    | The session hit a live Claude API error (500 / 529 / 429). Transient — clears when it recovers. The HTTP code is a DETAIL refinement, not part of the status (§ 2, #584). |
| `stale`    | No recent report **and the machine has no watcher**, so the true state is unknown. Shown (grey, dotted `◌`) instead of a false `ended`: *no news* ≠ *dead*. Resolves once a watcher runs there (#284/#285). |
| `ended`    | The session is over (closed, or its process is gone).                   |

`waiting`, `stalled` and `error` are the three statuses that call the operator, and a session can raise a call of its own on top of any status ([ADR-0010](../adr/0010-session-raised-operator-call.md)). `waiting`
means *the operator is the blocker*; `stalled` means *a tool hung and the turn is
stuck*. Both are what the dashboard exists to surface — the sessions that need a
human right now.

---

## 1bis. What a status colour says

Every status is drawn as a coloured pill in both clients. **A status colour states
how much attention the session requires, and nothing else.**

The nine colours were picked one at a time, as each status was added, and the rule
above was never written — so three of them spent three vivid, unrelated hues on
three ways of saying *running, leave it alone*. In front of twenty sessions the
operator could not answer the one question they opened vigie for by looking; they
had to read every label. Colour did no sorting (#654).

Four families, four colours:

| Family | Statuses | Light | Dark |
| --- | --- | --- | --- |
| **Running** — nothing to do | `working`, `thinking`, `compacting` | `#16a34a` | `#4ade80` |
| **Calling** — it needs a human | `waiting` | `#b45309` | `#fbbf24` |
| | `stalled` | `#ea580c` | `#fb923c` |
| | `error` | `#dc2626` | `#f87171` |
| **At rest** — between turns | `idle` | `#2563eb` | `#60a5fa` |
| **Over** — nothing more will happen | `stale`, `ended` | `#94a3b8` | `#64748b` |

The calling family keeps three colours because they are three different asks:
answer a prompt, unstick a tool, look at an outage. The other families ask for
nothing, so they need no distinction between their members.

**`stale` and `ended` share the grey deliberately.** Under the rule they are one
family. Telling them apart is a *second* reading and is carried by shape — `◌`
against `●` — which is what § 1 already asked for. A second grey separates
nothing at a glance and costs the rule its meaning.

**The pulse belongs to the calling family and stays there.** It runs on the state
pill for a degraded chain ([sessions-chrome.md](sessions-chrome.md) § 6); on a
status it would mean *this one is asking*. Extending it to a running status would
make the calm side of the board shout.

### The label is the redundancy, colour is the accelerator

The green-versus-warm axis carries the whole rule, and that is the most common
colour deficiency there is. The status pill nevertheless carries no shape of its
own, and that is deliberate: **the status is written out beside the dot**, in the
terminal and in the browser alike. Nothing here is knowable by colour only.

An operator who cannot separate the hues reads the words — which is what everyone
did before this rule existed. They lose the acceleration, not the answer. Giving
nine statuses nine shapes would slow the column down for everyone in order to
speed it up for some.

This is the opposite call from the state pill, which encodes its level in shape as
well as colour ([sessions-chrome.md](sessions-chrome.md) § 3) — because that pill
has no label beside it. The difference is the label, not the audience.

---

## 2. Two sources decide status

A session's status is fed by two independent observers. Neither is authoritative
alone; the server reconciles them (§ 3).

**Hooks — precise, event-driven.** Claude Code fires lifecycle hooks that report
a status the moment it changes:

| Hook event         | Status it sets                          |
| ------------------ | --------------------------------------- |
| `SessionStart`     | `idle` if the session was unknown, or if it was `ended` (a resume — see below); otherwise it changes nothing |
| `UserPromptSubmit` | `working`                               |
| `Notification`     | `waiting` on `permission_prompt` (a human must decide); `idle` on `idle_prompt` (finished, awaiting the next prompt) — split by the payload's `notification_type` |
| `Stop`             | `idle`                                  |
| `SessionEnd`       | `ended`                                 |

**A `SessionStart` on an `ended` session reopens it**, clearing the end time with
it. `claude --resume` keeps the session id, so the same row comes back; the event
is the only signal that says the session is not over, and treating it as
"changes nothing" left the board announcing an end that had already been undone —
until the operator typed something, which is a correction the board should not
need (#664).

The rule stays narrow: for every *live* status, `SessionStart` still says a
session exists rather than what it is doing, and changes nothing. `ended` is the
one value the event contradicts.

`waiting` is a *semantic* state — Claude asked a question — and it leaves no
trace in the transcript, so **nothing can infer it from transcript activity**. A
hook is told, and the registry below is told. Neither is a guess, which is what
§ 3 turns on.

**The watcher — coverage, poll-derived.** A background watcher scans local
transcripts every few seconds and reports a status for every recent session,
including ones the hooks never covered (already-open sessions, or a machine where
hooks aren't installed). It derives status from three signals — Claude Code's own
**session registry**, whether the session's **process is alive**, and whether its
**transcript is changing**.

**The registry wins where it covers the session** (#254). Claude Code maintains
it for its live sessions, so it states what the transcript can only be read for,
and the heuristic below is not consulted at all:

- `busy` → `working`; `idle` or `shell` → `idle`; `waiting` → `waiting`, carrying
  its `waitingFor` reason into DETAIL. An unrecognised value degrades to `idle`: a
  live session is never a false `ended`. The enum is closed
  ([ADR-0008](../adr/0008-compacting-status.md));
- the registry's `{PID, procStart}` says the backing process is gone → `ended`.

**The transcript heuristic covers the rest** — a session the registry does not
list, typically an older Claude Code:

- process gone → `ended` (reliable even on a hard kill);
- process alive but running a **newer** session id (same `{PID, procStart}`, a
  different id now in the registry) → `ended` for the old id: the session was
  superseded in place, typically by `/clear`. Without this the old transcript,
  still fresh and backed by the reused process, would linger as a ghost `idle`
  row (see [session-lineage.md](session-lineage.md), #367);
- transcript written just now, or the last turn stopped mid-tool-call and a tool
  may still be running → `working`;
- process alive but quiet → `idle`, for any idle duration (a long idle session
  is not "gone");
- no process mapping and quiet → `ended` (presumed closed — a live session would
  have registered a mapping);
- last assistant line is an API error (`isApiErrorMessage` in the transcript) →
  `error`, carrying the HTTP code; a later non-error line clears it;
- last assistant block is a `thinking` block → `thinking` (reasoning before any
  text/tool output); a later text/tool line clears it. A heuristic: at rest a
  finished turn ends with text/tool, so this reads true only mid-turn.
- a foreground `tool_use` with no matching `tool_result` (paired by id) while the
  session is otherwise quiet and idle → `stalled` (the tool hung, the turn is
  parked). An unresolved *background* Bash (`run_in_background`) instead keeps the
  session `working` — a real background task, not a hang.

  **The pairing does not replace the 5-minute tool window, it completes it.** The
  window is the grace period: a tool call may legitimately run that long, so the
  session reads `working` throughout — without it a build, a test suite or a long
  search would all be reported as a hung tool within 45 s, a false positive on one
  of the statuses that call the operator. What the pairing adds is what happens
  *after* the window: before it, a turn stopped on a tool simply fell to `idle`
  once the window elapsed, so a hung tool looked exactly like a finished turn. Now
  it reads `stalled`.

  Measured on a transcript frozen on an unanswered `tool_use`: `working` at 5 s,
  30 s, 1 min and 3 min; `stalled` at 6 min. An earlier version of this section
  said the pairing *replaced* the window, which would have meant `stalled` at 45 s
  for every slow tool (#530).

  **The pairing is scoped to the turn: a real user prompt closes every older
  unresolved `tool_use`.** A result that never arrives — Claude Code killed while
  a tool was in flight — otherwise pins the session to `stalled` for the rest of
  its life, at every pause between turns, and no operator action can clear it
  (vigie is observe-only, [ADR-0005](../adr/0005-observe-only.md)). A prompt is
  proof the session moved on, so a tool call from before it cannot be what the
  current turn is parked on. Only a prompt the *operator* typed counts: Claude
  Code injects `user` lines of its own for system reminders, skill preambles and
  the "Continue from where you left off." resume, and marks them `isMeta` — those
  land in the middle of a live tool call and must not close it (#483).

The watcher can see `working`, `thinking`, `compacting`, `idle`, `stalled`,
`ended`, `error` — and `waiting`, but only where the registry states it. What it
**cannot** do is *infer* `waiting`: to the watcher a permission prompt and a
running tool are the same frozen transcript, which is the whole subject of § 3.
`compacting` is the one it does not derive alone: the `PreCompact` hook opens it
and the watcher closes it from the transcript's `compact_boundary`
([ADR-0008](../adr/0008-compacting-status.md)).

**DETAIL refinements (no status change).** A few signals annotate the activity
column without touching the base status — a lighter touch than a full status when
the state itself is unchanged:

- a `shell` registry status keeps the session `idle` and sets DETAIL to `shell`
  (dropped to a shell prompt) (#280);
- a synthetic `[Request interrupted by user]` / `[Request interrupted by user for
  tool use]` `user` line — the last non-system message — keeps the base `idle`
  but sets DETAIL to `interrupted`, so a turn the operator killed mid-flight is
  distinguishable from one that finished cleanly. It clears with no timer: the
  next real user prompt or assistant message replaces it (#351). The synthetic
  line carries its `content` as a block array, where a typed prompt is a plain
  string, so a user typing that literal text does not false-positive;
- the **HTTP code of a live API error** keeps the status `error` and sets DETAIL
  to the named code — `529 Overloaded`, or the bare number when it is not one the
  client names. It used to be appended inside the status cell (`● error 529`),
  which made `error` the only status carrying a refinement in its own cell; it
  now follows the same rule as the two above (#584).

**DETAIL has one occupant, so it has an order.** The watcher writes the current
activity there, and two refinements can want the cell at once. The precedence is
stated rather than left to whichever branch runs first:

**a raised call > an API error > the activity.**

A call is why the row is blinking ([ADR-0010](../adr/0010-session-raised-operator-call.md), #389)
and outranks everything. An API error outranks the activity because once the API
answers 529 the tool that ran last is of no interest, and the code is the only
thing separating an outage from throttling.

The label is computed when the cell is rendered, never written into DETAIL by the
watcher: DETAIL is persisted and cleared on a status change, so storing the code
there as text would be a second source of truth for what `api_error_status`
already carries.

---

## 3. Reconciliation rules

The two sources can disagree; the server resolves it by **observer authority**,
not a table of status pairs. Each session remembers *which* observer last set
its status (its **source**: `hook` or `watch`).

**The watcher is authoritative for what it can positively observe** — `working`,
`thinking`, `error`, `ended` — and any such *change* wins and becomes watch-owned.
A report that merely **confirms** the current status keeps the current owner: a
confirmation is not a change, so it never transfers ownership away from a hook.

**A hook is authoritative for what only it can see** — that the operator is the
blocker (`waiting`), or that a turn is open while Claude works silently. The
watcher only ever sees a quiet-but-alive session as `idle`, so its `idle` must
**not** retract a *hook-owned* `waiting`, `working`, or `thinking`. A hook `Stop`
(→ `idle`) or new activity ends the turn.

**A `waiting` is only cleared once the transcript moves.** To the watcher, "a
tool is running" and "a permission prompt is blocking" look identical — a turn
stopped on a tool call with a frozen transcript. So **any** status it infers from
that silence — `working`, `thinking`, `compacting`, `stalled` — may not clear a
hook `waiting` until the transcript has actually changed past when waiting was
posted (the report's timestamp is the transcript mtime). `error` and `ended` are
positive observations and still win.

The rule is stated as a *deny* list, and the code implements it as one: it was
once an allow list naming three statuses, so `stalled` fell through it when that
status was added and a permission prompt read as a hung tool for the rest of the
session. A status added later is held by default — a late release costs less than
naming the wrong cause (#508).

**The watcher must retract its own stale state.** The key consequence: a `working`
that the *watcher itself* set (a hooks-free session) falls back to `idle` when the
transcript goes quiet — because it is watch-owned, not hook-owned. Without the
source, a blanket "keep working" latches a finished session on `working` forever.

**A session that stops reporting becomes `ended` or `stale`, depending on who
else went quiet.** The watcher re-reports every scan, so a live session is
refreshed constantly. If more than ~60 s pass with no report, the answer turns on
whether that machine's watcher is still beating:

- **its watcher is up** → `ended`. A running watcher would have kept a live
  session fresh, so silence there means the session is genuinely gone. This
  catches what no explicit event covers: the session dropped out of the scan
  window, or its process died without a `SessionEnd`;
- **no watcher heartbeat from that machine** → `stale`. Nothing is observing it,
  so silence says *unobserved*, not *dead*, and an unknown beats a false `ended`
  (§ 1, #284/#285). It resolves by itself once a watcher runs there.

A session already `ended` stays `ended`.

**`error` overrides `working`/`idle`, never `ended`.** A live session that just
hit an API error shows `error`; a closed session stays `ended`, so a stale
transcript never lingers red. `error` is transient by construction — the next
non-error line restores `working`/`idle`, so a recovered retry is not held red.

---

## 4. Why not one source

Hooks are precise but incomplete: they only cover sessions started with the
hooks installed, and a crash can skip `SessionEnd`. The watcher is complete but
coarse: it sees activity and liveness, never intent. Together — hooks for
`waiting` and instant transitions, the watcher for coverage and a liveness
backstop — they cover every session without either being a single point of
failure. The reconciliation rules (§ 3) resolve the overlap in favor of the
more-informed source.

---

## 5. Detection sources & reliability

Not every status is equally trustworthy. What produces each one, and how much to
trust it:

| Status     | Source(s) | Reliability |
| ---------- | --------- | ----------- |
| `working`  | hook `UserPromptSubmit`; watcher (recent transcript writes / a live `tool_use` turn) | **Reliable.** A hook pins it instantly; the watcher covers hooks-free sessions. Held through a quiet turn by authority (§ 3). |
| `waiting`  | hook `Notification`; watcher (Claude Code's session registry) | **Reliable from either, and never inferred.** The hook is instant; the registry is read at the next scan and covers a machine with no hooks installed. What no source does is *derive* it from a quiet transcript — a permission prompt and a running tool look identical there (§ 3). |
| `idle`     | hook `Stop`; watcher (alive + quiet) | **Reliable.** Distinguishing `idle` from `ended` rests on process presence (PID + `/proc`, [ADR-0006](../adr/0006-session-presence-via-proc.md)). |
| `ended`    | hook `SessionEnd`; watcher (dead process, or no mapping + quiet); server (no report for ~60 s **on a machine whose watcher is still beating**) | **Reliable** for a hooked end or a dead process; a **heuristic** for a hooks-free session that simply went quiet. |
| `stale`    | server (no report for ~60 s and no watcher heartbeat from that machine) | **Reliable as an admission, not as a diagnosis.** It says only that nothing is observing the machine — the session may be alive, finished, or gone. That is the point: an unknown beats a false `ended` (§ 1, #284/#285). |
| `error`    | watcher (`isApiErrorMessage` in the transcript) | **Reliable signal, sampled.** The flag is unambiguous, but surfaced only at the next scan, not instantly (Claude Code has no error hook). |
| `thinking` | watcher only (last content block is a `thinking` block) | **Best-effort heuristic.** No hook signals reasoning; it is inferred from the transcript, sampled every ~2 s, invisible in a hooks-only deployment, and can briefly mis-read when a `tool_use` block follows the thinking block. |
| `compacting` | hook `PreCompact` opens it; watcher closes it on `compact_boundary` | **Reliable at both ends, sampled at the close.** The open is a hook, so it is instant; the close is read from the transcript at the next scan. Invisible in a hooks-only deployment, since nothing would close it. |
| `stalled`  | watcher only (unresolved `tool_use`↔`tool_result` + quiet) | **Reliable signal, sampled.** The pairing is exact (an id match), not a timeout guess. It surfaces once the tool window has elapsed — a tool call is allowed to run that long before silence means anything (§ 2), so the signal is deliberately late rather than wrong. Self-healing: a tool whose result never arrives is closed by the next operator prompt rather than pinning the session (§ 2). Invisible in a hooks-only deployment. |

**Decision on `thinking` (#207): kept, as an explicit best-effort refinement.**
Dropping it would lose a genuine, if imperfect, signal; hardening it to real-time
is impossible without a Claude Code "thinking" hook, which does not exist. It is
therefore documented here as best-effort and only ever *refines* an active turn
(`working`/`idle`) — never `waiting`, `error`, or `ended` — so a wrong guess is
always a near-miss, not a misleading state.

---

## Appendix — doc conventions

`docs/design/` = the *what* (user-observable); `docs/adr/` = decisions with
rationale; code = the *how*. Docs never paraphrase code. Adding/modifying design
docs requires explicit consent — propose first.
