# ADR-0005: Observe-only — no downstream control channel into sessions

## Status

Accepted.

## Context

claude-fleet reports the state of Claude Code sessions across machines. A
natural temptation is to also *act* on sessions — e.g. remote control (`/rc`)
asks not only "is this session remotely piloted?" but "turn that on from the
dashboard". Turning it on means executing a command **inside** a running
session: a downstream control channel from claude-fleet into Claude Code.

An early implementation already crossed this line: rc was a hand-set boolean
flag with a `c` toggle and a `POST` endpoint, pretending to control something it
did not. It confused a *wish* with the *real* state and satisfied neither.

Building a real downstream channel has no clean path: Claude Code's daemon is
transient with an internal, key-authenticated control socket (fragile to
reverse-engineer, breaks on updates), and the only "raw" alternative — injecting
keystrokes into a session's PTY — is hacky and can corrupt live input.
Controlling sessions is also a fundamentally different concern from observing
them, with a different security posture.

## Decision

**claude-fleet is observe-only.** It reads and reports session state; it never
writes into a session or drives one.

- Session facts are **detected**, never operator-set. Remote control is read
  from `bridgeSessionId` in the Claude session file (see
  [`docs/design/remote-control.md`](../design/remote-control.md)) — a read-only
  signal.
- Piloting a session stays with the native tools: `/rc` in the terminal, the web
  app, the mobile app. claude-fleet shows *that* it is on; it does not toggle it.
- No PTY injection, no reverse-engineered control socket, no write path into a
  session.

## Consequences

- The rc toggle (`c` key, `POST /api/sessions/{id}/rc`, the stored/settable flag)
  is **removed**; the `RC` column becomes a detected, read-only state.
- The client→server write path introduced for rc is dropped (the server API
  returns to report + read-only). If a legitimate server-side *setting* needs a
  write later (e.g. retention already uses `POST /api/settings`), that is a
  server config write, not a session control channel — this ADR is about not
  writing **into sessions**.
- Robustness and security: the fleet does not break when Claude Code internals
  change, and offers no surface for injecting into sessions.
- Any future "do something to a session" feature must first be reconciled with
  this ADR (amend it explicitly, or don't build the feature).
