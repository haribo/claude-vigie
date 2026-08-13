# Remote control — Design Specification

**Status:** Accepted. rc is detected and read-only; activation is out of scope
per [ADR-0005](../adr/0005-observe-only.md).

Source of truth for what "remote control" (rc) means in vigie and how the
operator interacts with it — the user-observable behavior, not the code.

---

## 1. What rc is

Remote control is Claude Code's **`/rc` slash command**: running it in a session
lets that session be driven from the web (claude.ai) or the Claude Android app.
vigie **does not pilot anything** — it only reflects, and (if decided,
§ 4) triggers, `/rc` on the session itself.

---

## 2. Real state (detection) — decided

A session is remotely controlled **iff its Claude session file has a non-empty
`bridgeSessionId`**.

- Source: `~/.claude/sessions/<pid>.json`, written by Claude Code, e.g.
  `{"sessionId": "...", "name": "web-app", "status": "idle", "bridgeSessionId": "session_014wi…"}`.
- `bridgeSessionId` present → `/rc` active; absent/empty → not.
- The **resume URL** is `https://claude.ai/code/<bridgeSessionId>` — the same link
  Claude prints in the session and writes verbatim as the `url` field of the
  transcript's `bridge_status` lines. It is surfaced (§ 3) so the operator can open
  the controlled session; it clears with the flag when `/rc` goes off (#253).
- Verified against six live sessions: those carrying a bridge read `rc on`, those
  without read `rc off` — matching what the operator sees in each session's
  terminal footer.

This **replaced** the earlier hand-set boolean flag (the `c` toggle), which was
disconnected from reality: the watcher now reports the detected state on every
scan, and the `c` toggle and its write endpoint are gone.

> Note: `~/.claude/sessions/<pid>.json` also exposes `sessionId`, `cwd`, `name`,
> and `status` directly — a cleaner source than parsing transcripts. Whether to
> adopt it for those fields too is out of scope here (separate design).

---

## 3. Display

- The `RC` column shows the **detected** state: active vs inactive.
- The **resume URL** appears in the session detail — a plain "Remote" line in the
  TUI, a clickable link in the web dashboard — only while rc is active.
- It is read-only with respect to reality — it never shows an operator wish that
  isn't true on the session.

---

## 4. Activation is out of scope

vigie does **not** turn `/rc` on or off. Activation means executing a
command inside a running session — a downstream control channel that
[ADR-0005](../adr/0005-observe-only.md) rules out (vigie is observe-only).
The operator activates rc with the native tools: `/rc` in the session terminal,
or the web / mobile app. vigie shows *that* it is on; it never toggles it.

---

## Appendix — doc conventions

`docs/design/` = the *what* (user-observable); `docs/adr/` = decisions with
rationale; code = the *how*. Docs never paraphrase code. Adding/modifying design
docs requires explicit consent — propose first.
