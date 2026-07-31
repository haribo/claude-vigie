# Session status — Design Specification

**Status:** Accepted.

Source of truth for what a session's **status** means in claude-fleet and what
makes it change — the user-observable behavior, not the code. Like everything in
claude-fleet, status is **detected**, never operator-set
([ADR-0005](../adr/0005-observe-only.md)).

---

## 1. The six statuses

Every session shows exactly one status. What each tells the operator:

| Status     | Meaning                                                                 |
| ---------- | ---------------------------------------------------------------------- |
| `working`  | Claude is actively producing — a turn is running.                       |
| `thinking` | Claude is reasoning inside a turn — extended thinking, before it outputs text or a tool call. A sub-state of an active turn. |
| `waiting`  | Claude has stopped and is **waiting on the human** (a prompt or permission). |
| `idle`     | The session is open and alive but between turns — nobody is acting.     |
| `error`    | The session hit a live Claude API error (500 / 529 / 429). Transient — clears when it recovers. |
| `ended`    | The session is over (closed, or its process is gone).                   |

`waiting` is the one status that carries intent: it means *the operator is the
blocker*, not Claude. It is what the dashboard exists to surface — the session
that needs a human right now.

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

The watcher can see `working`, `thinking`, `idle`, `ended`, and `error`. It
**cannot** see `waiting`.

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

**Decision on `thinking` (#207): kept, as an explicit best-effort refinement.**
Dropping it would lose a genuine, if imperfect, signal; hardening it to real-time
is impossible without a Claude Code "thinking" hook, which does not exist. It is
therefore documented here as best-effort and only ever *refines* an active turn
(`working`/`idle`) — never `waiting`, `error`, or `ended` — so a wrong guess is
always a near-miss, not a misleading state.

---

## Appendix — doc conventions (from tribnest)

`docs/design/` = the *what* (user-observable); `docs/adr/` = decisions with
rationale; code = the *how*. Docs never paraphrase code. Adding/modifying design
docs requires explicit consent — propose first.
