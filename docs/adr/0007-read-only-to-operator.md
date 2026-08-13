# ADR-0007: vigie is read-only to the operator — no session-handling state

## Status

Accepted.

## Context

ADR-0005 established that vigie has no **outbound** control channel: it never
writes into a session. This ADR covers the **inbound** half — operator state.

vigie is a dashboard the operator *reads*; the operator *acts* in the session
itself (their own terminal). A tempting pattern is to also let vigie hold state
about how the operator is *handling* a session — an inbox: mark-read, acknowledge,
dismiss, snooze, resolve, pin.

An early implementation crossed this line: the unread marker (#259). To move a
session from `unread → read` you had to **open it in vigie**. But the real
handling happens in the session, not in vigie — so the state was decoupled from
reality: you could "acknowledge" in vigie without touching the session, and clear
a real waiting prompt in the session while vigie still showed it unread. It was an
inbox interaction bolted onto a mirror.

Together with ADR-0005 this makes vigie a one-way mirror: no control flows out
into sessions, and no handling-state flows in from the operator.

## Decision

**vigie is read-only to the operator.** It holds no state that represents the
*handling* of a session, and no feature may require an action *on vigie* to reach
a correct state.

The litmus test for any future feature:

> *To make this indicator correct, or to clear it, must I do something to vigie?*

If yes, the feature is forbidden — the handling belongs in the session.

### The line, drawn explicitly

- **Allowed** — shaping your *view*: sort, filter, group, enter a detail, navigate
  (the `n` jump), even persisted (e.g. the saved sort order, #237); and *outbound*
  signals that reach the operator without holding state: desktop notifications
  (#260).
- **Forbidden** — any *handling* state about a session held in vigie:
  unread / ack / read, mark-handled, dismiss / snooze, resolve / acknowledge, pin.

## Consequences

- The unread machinery (#259) is **removed**: the gutter dot, the bold session
  name, the `● N unread` summary counter, and the `seen`/`ack`/`unread` state.
  This supersedes #259 and makes #281 (a rendering fix for it) moot.
- The `n` hotkey (#261) stays as **pure navigation** — it jumps to the oldest
  session in an attention state; it no longer acknowledges anything.
- "Don't miss anything" now rests entirely on the outbound signals: desktop
  notifications (#260) and the `n` jump-to-next (#261). These become load-bearing.
- Any future "the operator did something to a session" state must first be
  reconciled with this ADR (amend it explicitly, or don't build the feature).

## Related

- [ADR-0005](0005-observe-only.md) — the outbound half (no control channel into sessions).
- [ADR-0010](0010-session-raised-operator-call.md) — a session-authored signal tested
  against the litmus above and allowed: it is set *and* cleared by the session, so
  no action on vigie is ever required.
