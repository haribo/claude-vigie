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
| `error`    | The session hit a live Claude API error (500 / 529 / 429). Transient — clears when it recovers. |
| `stale`    | No recent report **and the machine has no watcher**, so the true state is unknown. Shown (grey, dotted `◌`) instead of a false `ended`: *no news* ≠ *dead*. Resolves once a watcher runs there (#284/#285). |
| `ended`    | The session is over (closed, or its process is gone).                   |

`waiting` and `stalled` are the two statuses that call the operator: `waiting`
means *the operator is the blocker*; `stalled` means *a tool hung and the turn is
stuck*. Both are what the dashboard exists to surface — the sessions that need a
human right now.

---

## 2. Two sources decide status

A session's status is fed by two independent observers. Neither is authoritative
alone; the server reconciles them (§ 3).

**Hooks — precise, event-driven.** Claude Code fires lifecycle hooks that report
a status the moment it changes:

| Hook event         | Status it sets                          |
| ------------------ | --------------------------------------- |
| `SessionStart`     | `idle` (only if the session was unknown) |
| `UserPromptSubmit` | `working`                               |
| `Notification`     | `waiting` on `permission_prompt` (a human must decide); `idle` on `idle_prompt` (finished, awaiting the next prompt) — split by the payload's `notification_type` |
| `Stop`             | `idle`                                  |
| `SessionEnd`       | `ended`                                 |

Only a hook can observe `waiting`: it is a *semantic* state (Claude asked a
question) with no visible trace in the transcript, so nothing else can infer it.

**The watcher — coverage, poll-derived.** A background watcher scans local
transcripts every few seconds and reports a status for every recent session,
including ones the hooks never covered (already-open sessions, or a machine where
hooks aren't installed). It derives status from two signals — is the session's
**process alive**, and is its **transcript changing**:

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
  session `working` — a real background task, not a hang. This exact pairing
  replaces the old blind 5-minute "a tool may still be running" window.

The watcher can see `working`, `thinking`, `idle`, `stalled`, `ended`, and
`error`. It **cannot** see `waiting`.

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
  string, so a user typing that literal text does not false-positive.

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
stopped on a tool call with a frozen transcript. So its inferred `working` may
not clear a hook `waiting` until the transcript has actually changed past when
waiting was posted (the report's timestamp is the transcript mtime). `error` and
`ended` are positive observations and still win.

**The watcher must retract its own stale state.** The key consequence: a `working`
that the *watcher itself* set (a hooks-free session) falls back to `idle` when the
transcript goes quiet — because it is watch-owned, not hook-owned. Without the
source, a blanket "keep working" latches a finished session on `working` forever.

**A session that stops reporting becomes `ended`.** The watcher re-reports every
scan, so a live session is refreshed constantly. If more than ~60 s pass with no
report, the session is shown as `ended` — this catches the cases no explicit
event covers: the watcher process died, the machine went offline, or a session
dropped out of the scan window. A session already `ended` stays `ended`.

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
| `waiting`  | hook `Notification` only | **Reliable when hooks are installed, else invisible.** Only a hook can see that the operator is the blocker; the watcher never infers it. |
| `idle`     | hook `Stop`; watcher (alive + quiet) | **Reliable.** Distinguishing `idle` from `ended` rests on process presence (PID + `/proc`, [ADR-0006](../adr/0006-session-presence-via-proc.md)). |
| `ended`    | hook `SessionEnd`; watcher (dead process, or no mapping + quiet); server (no report for ~60 s) | **Reliable** for a hooked end or a dead process; a **heuristic** for a hooks-free session that simply went quiet. |
| `error`    | watcher (`isApiErrorMessage` in the transcript) | **Reliable signal, sampled.** The flag is unambiguous, but surfaced only at the next scan, not instantly (Claude Code has no error hook). |
| `thinking` | watcher only (last content block is a `thinking` block) | **Best-effort heuristic.** No hook signals reasoning; it is inferred from the transcript, sampled every ~2 s, invisible in a hooks-only deployment, and can briefly mis-read when a `tool_use` block follows the thinking block. |
| `stalled`  | watcher only (unresolved `tool_use`↔`tool_result` + quiet) | **Reliable signal, sampled.** The pairing is exact (an id match), not a timeout guess; surfaced at the next scan once the session has been quiet past a short threshold. Invisible in a hooks-only deployment. |

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
