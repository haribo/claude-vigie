# Remote control — Design Specification

**Status:** Detection decided; activation (send `/rc`) is an open decision (§ 4).

Source of truth for what "remote control" (rc) means in claude-fleet and how the
operator interacts with it — the user-observable behavior, not the code.

---

## 1. What rc is

Remote control is Claude Code's **`/rc` slash command**: running it in a session
lets that session be driven from the web (claude.ai) or the Claude Android app.
claude-fleet **does not pilot anything** — it only reflects, and (if decided,
§ 4) triggers, `/rc` on the session itself.

---

## 2. Real state (detection) — decided

A session is remotely controlled **iff its Claude session file has a non-empty
`bridgeSessionId`**.

- Source: `~/.claude/sessions/<pid>.json`, written by Claude Code, e.g.
  `{"sessionId": "...", "name": "tribnest", "status": "idle", "bridgeSessionId": "session_014wi…"}`.
- `bridgeSessionId` present → `/rc` active; absent/empty → not.
- Verified: tribnest, plain-note, claude-fleet carry a bridge (rc on); shellf,
  melonia, sirius do not (rc off) — matching what the operator sees in each
  session's terminal footer.

This **replaces** the current hand-set boolean flag (the `c` toggle), which was
disconnected from reality and must be removed.

> Note: `~/.claude/sessions/<pid>.json` also exposes `sessionId`, `cwd`, `name`,
> and `status` directly — a cleaner source than parsing transcripts. Whether to
> adopt it for those fields too is out of scope here (separate design).

---

## 3. Display

- The `RC` column shows the **detected** state: active vs inactive.
- It is read-only with respect to reality — it never shows an operator wish that
  isn't true on the session.

---

## 4. Activation (send `/rc`) — open decision

The operator also wants to turn rc on from claude-fleet, which means **executing
`/rc` inside the target session**. There is **no clean channel** for this:

- The Claude daemon is transient (exits when idle) and its control socket uses an
  internal, key-authenticated protocol — reusing it means fragile reverse
  engineering that breaks on Claude Code updates.
- The only "raw" option is injecting `/rc\n` into the session's PTY
  (`/dev/pts/N`), which is fragile and can corrupt whatever the operator is
  typing.

**Options to decide:**

| Option | What it gives | Cost |
|--------|---------------|------|
| A — detection only | The `RC` column shows real `/rc` state; operator runs `/rc` by hand in the terminal | none extra; honest but no remote action |
| B — PTY injection | claude-fleet types `/rc` into the session's terminal | hacky, fragile, can clash with live input |
| C — wait for a supported channel | Do it right if Claude Code exposes one | rc activation deferred |

Recommended split: ship **§ 2 detection now** (real, clean), decide activation
(A/B/C) separately.

---

## Appendix — doc conventions (from tribnest)

`docs/design/` = the *what* (user-observable); `docs/adr/` = decisions with
rationale; code = the *how*. Docs never paraphrase code. Adding/modifying design
docs requires explicit consent — propose first.
