# Session status — Design Specification

**Status:** Accepted.

Source of truth for what a session's **status** means in claude-fleet and what
makes it change — the user-observable behavior, not the code. Like everything in
claude-fleet, status is **detected**, never operator-set
([ADR-0005](../adr/0005-observe-only.md)).

---

## 1. The four statuses

Every session shows exactly one status. What each tells the operator:

| Status    | Meaning                                                                 |
| --------- | ---------------------------------------------------------------------- |
| `working` | Claude is actively producing — a turn is running.                       |
| `waiting` | Claude has stopped and is **waiting on the human** (a prompt or permission). |
| `idle`    | The session is open and alive but between turns — nobody is acting.     |
| `ended`   | The session is over (closed, or its process is gone).                   |

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
| `Notification`     | `waiting` — Claude is asking for input  |
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
  have registered a mapping).

The watcher can see `working`, `idle`, and `ended`. It **cannot** see `waiting`.

---

## 3. Reconciliation rules

The two sources can disagree; two rules keep the result honest.

**`waiting` is sticky over the watcher's `idle`.** When a hook has set `waiting`,
the watcher — which only ever sees that same session as `idle` (alive, quiet) —
must not overwrite it. `waiting` persists until real activity resumes (back to
`working`) or the session ends. Without this, every watcher scan would erase the
"needs a human" signal a second after the hook set it.

**A session that stops reporting becomes `ended`.** The watcher re-reports every
scan, so a live session is refreshed constantly. If more than ~60 s pass with no
report, the session is shown as `ended` — this catches the cases no explicit
event covers: the watcher process died, the machine went offline, or a session
dropped out of the scan window. A session already `ended` stays `ended`.

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

## Appendix — doc conventions (from tribnest)

`docs/design/` = the *what* (user-observable); `docs/adr/` = decisions with
rationale; code = the *how*. Docs never paraphrase code. Adding/modifying design
docs requires explicit consent — propose first.
