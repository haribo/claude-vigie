# ADR-0010: A session may raise an explicit call for the operator

## Status

Accepted.

## Context

Everything vigie shows today is **inferred**: status comes from Claude Code's own
registry, the transcripts, and the hooks. What cannot be inferred is *intent* —
"the thing you asked for is done, come and look".

The operator's only proxy for that is `idle`, which is ambiguous by construction:
a session is idle between two turns, after a one-line answer, and after the work
that actually mattered. Waiting for the right `idle` means watching all of them.

The wanted flow is direct: the operator writes something like *"when you're
finished, tell me in vigie"*, Claude runs a command at the end of its turn, and
vigie surfaces the call until work resumes in that session.

That stores state **about a session, inside vigie** — the exact shape
[ADR-0007](0007-read-only-to-operator.md) exists to forbid. It must be reconciled
before any code is written.

## Decision

**A session may raise a `call`: a signal the session authors itself, which vigie
displays until the session itself retracts it.**

- **Set by the session** — a command run inside it, at the end of a turn.
- **Cleared by the session** — `UserPromptSubmit` when work resumes there, and
  `SessionEnd`.
- **No action on vigie is ever required** to reach a correct state.

### Why this passes ADR-0007

ADR-0007 forbids **handling** state: state whose truth lives in the operator's
head and can only be made true by acting *on vigie* — unread/ack, dismiss, snooze,
resolve, pin. The unread marker (#259) failed precisely there: moving a session
from `unread → read` required opening it **in vigie**, so the marker could drift
away from what was true in the session.

The call inverts that ownership. It is authored by the session and retracted by
the session; its truth lives where the work lives. Applying ADR-0007's own litmus —

> *To make this indicator correct, or to clear it, must I do something to vigie?*

— the answer is **no**: it is cleared by working in the session, which is exactly
where ADR-0007 says handling belongs. The call is therefore an **observed signal**,
in the same family as status or activity, not operator state.

**ADR-0007 stands unamended.** This ADR applies its rule rather than bending it,
and sets the shape any future session-authored signal must take: authored *and*
retracted by the session. Anything that needs clearing on vigie remains forbidden.

### Why ADR-0005 is untouched

The flow is session → vigie. Nothing travels from vigie into a session: the
command runs *inside* the session, on the session's own initiative, with vigie as
the destination. The observe-only decision
([ADR-0005](0005-observe-only.md)) is unaffected — vigie remains a destination for
signals, never a driver of sessions.

## Rejected alternatives

| Alternative | Why not |
| --- | --- |
| Acknowledge / dismiss in the TUI | Forbidden by ADR-0007 — clearing would require acting on vigie. This is exactly what removed the unread marker (#259). |
| A dedicated `call` status | The registry status enum is closed ([ADR-0008](0008-compacting-status.md)), and the call is orthogonal to status: a calling session is still `idle`. |
| A dedicated colour for the call | Another hue in a palette that is already dense and whose colours *are* the status vocabulary — and it would contradict the status shown on the same row. The call borrows the status colour instead. |
| A pulsed marker in the left gutter | That margin already carries the selection cursor (`▎`, `renderRow`); the two collide whenever the calling row is also the selected row. |
| A dedicated `CALL` column | A permanent column for a rare signal, and the inert markers on every other row dilute what should stand out. |
| Repainting the whole row | The call would replace the status although it is orthogonal to it, and it collides with the selected-row background. |
| Blinking the session name | `NAME` already carries the status colour, and it is the text one must *read* to identify the session — blinking it forces waiting for the right frame. |
| ANSI blink (SGR 5) | Ignored by most modern terminals: invisible for some users, with no way for them to know. |

### Deferred, not rejected

Making the signal deterministic by having Claude **arm** the request and the
`Stop` hook **post** the call when the turn actually ends. Deferred as an extra
state round-trip for now; the natural-language instruction degrades gracefully
instead (a missed call costs an ordinary `idle`, which is the status quo).

## Consequences

- **The marker is a blinking `●` that keeps its status colour** — no new glyph, no
  new colour, no new column.
- **With blinking disabled**, a calling session is distinguishable only by its
  `DOING` message. Accepted: the call is an accelerator over the existing signals,
  not a replacement for them.
- **The call is orthogonal to status.** A calling session keeps whatever status it
  has; nothing in the status vocabulary changes.
- **The signal is best-effort.** It rests on Claude following an instruction, so a
  missed call degrades to today's behaviour rather than to a wrong display.
- **Unblocks the implementation** this ADR had to land before: #388 (the signal
  itself — command, storage, clearing, propagation), #389 (TUI), #390 (web),
  #391 (discoverability).

## Related

- [ADR-0005](0005-observe-only.md) — the outbound half: no control channel into sessions.
- [ADR-0007](0007-read-only-to-operator.md) — the inbound half this ADR must not break, and whose litmus it passes.
