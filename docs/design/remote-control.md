# Remote control — Design Specification

**Status:** Draft — pending decisions (§ 3). Do not implement until § 3 is resolved.

This document is the source of truth for what "remote control" (rc) means in
claude-fleet and how the operator interacts with it. It describes the intended
user-observable behavior, not the implementation.

---

## 1. Intent

The operator wants **two** capabilities on each session, together:

1. **See the real state** — whether a session *is actually* remote-controlled,
   detected automatically. Not a value the operator sets by hand.
2. **Turn it on/off** — enable or disable remote control for a session, from
   claude-fleet.

The current implementation only does a hand-set boolean flag (the `c` key),
disconnected from any real state. That satisfies neither intent fully and is the
source of the confusion this document exists to resolve.

---

## 2. Vocabulary to pin down

Before behavior, we must agree on what the words denote. These are **claims to
confirm or correct**, not established facts:

- **"Remote control" of a Claude Code session** — candidate meanings:
  - (a) the session is pilotable/piloted from **claude.ai** (web) — a session
    you started locally is attached to the cloud and can be driven remotely;
  - (b) the session is driven from the **mobile** app;
  - (c) a local mode you enable on the session;
  - (d) something else entirely.
- **"Activated"** — does turning rc on *do* something to the session (it becomes
  actually remotely pilotable), or is it only a marker/label in claude-fleet?

---

## 3. Open decisions (to resolve together)

Nothing is implemented until these are answered.

| # | Question | Why it blocks |
|---|----------|---------------|
| Q1 | What *is* remote control for a Claude session? (§ 2 — pick a/b/c/d) | Defines the whole feature |
| Q2 | How is the **real state** observable? Which source — the transcript, a session file, the `claude` process, a claude.ai API, a hook signal? | Determines what the watcher reads to detect it |
| Q3 | What does **enable/disable** concretely *do* to the session? A real effect, or a claude-fleet-side intent that something else honors? | Determines whether rc is a control or a label |
| Q4 | Who has authority when the **detected state** and the **operator toggle** disagree? Which wins, and for how long? | Determines the reconciliation rule |
| Q5 | Is rc **per session** or **per machine/account**? | Determines storage scope and the API shape |

---

## 4. To be filled once § 3 is decided

- **Detection** — the exact source and rule the watcher uses to read the real state.
- **Toggle** — what the on/off action sends, to whom, and its real effect.
- **Reconciliation** — how detected state and operator intent combine.
- **Display** — the `RC` column and any summary counter (states, colors).

---

## Appendix — how tribnest's docs are structured (adopted here)

`docs/design/` holds user-observable behavior (the *what*); `docs/adr/` holds
decisions with rationale (the *why*); code holds the *how*. Docs never
paraphrase code. This file is a design doc; the eventual detection/toggle
decision (Q1–Q5) will also get an ADR capturing the rejected alternatives.
