# ADR-0015: No timer decides what vigie cannot observe

## Status

Accepted (#810).

Generalises the reasoning of [ADR-0012](0012-retire-the-stalled-status.md) and
[ADR-0014](0014-retire-remote-control-detection.md), which applied it to one
subject each. Neither is superseded: both stand, and this states the rule they
share so the next case does not have to rediscover it.

## Context

vigie reports a session's state from outside it, and much of what it wants to say
is not directly observable. The recurring temptation is to fill the gap with a
duration: *this has been quiet long enough, therefore it is over.*

Three times now the answer has been no.

- **`stalled` (#256, retired by ADR-0012).** A tool call outstanding past a
  threshold was announced as a hung tool. The pairing proves a call is
  outstanding, never that it is stuck, and the verdict came from a timer over a
  duration only the operator can interpret. An hour-long test suite was reported
  as a fault.
- **Remote control (retired by ADR-0014).** Not a timer, the same error one step
  along: a *state* asserted from a signal that had stopped carrying it. Retired
  rather than rebuilt on the next undocumented field.
- **Backgrounded commands (#748, and this ADR).** A command that never reports
  its completion leaves the session reading `working`. The obvious guard is the
  subagents' 30-minute liveness window. It is the wrong guard here twice over: it
  keys on transcript **silence**, and silence is exactly what a background command
  produces, so it would release the very sessions the rule exists to catch — and
  it decides from a clock that a command has stopped, which is `stalled` again.

Recorded in `design/session-status.md` § 2 after #748. That was not enough: #810
arrived two days later proposing the cap again, on its own merits, and the
decision had to be re-argued from a paragraph inside a status specification. A
rule invoked three times deserves somewhere to be cited from.

## Decision

**vigie does not use elapsed time to decide something it cannot observe.**

A duration may describe (`SEEN` counts, the state modal ages a snapshot) and may
bound a *mechanism* whose failure is otherwise unbounded. It may not be the
evidence for a claim about a session's state.

Where the observation never comes, vigie holds the last thing it saw and says so,
rather than inventing an end.

### The price, stated

For work that outlives its call the price is concrete and measured, and it is
stated **twice, because one denominator flatters it.** Measured on the local corpus
of 95 transcripts, 2026-09-17:

| | |
|---|---|
| per launch | 19 of 866 — about **one in forty-six** |
| per session that launches any | 8 of 21 — about **one in two and a half** |

**Re-run it rather than trust it: `just residual`.** Until #843 these numbers came
from throwaway scripts that reimplemented the closing rules and no longer exist, so
no reader could re-derive the claim and disagree with it — which is how the figure
stayed wrong three times, each correction surfacing by accident during an unrelated
investigation. The tool reads the parser's own counts, so it follows the rules
instead of restating them, and it prints the corpus and the date every time.

**The attribution below is disputed and the count is not.** #842 finds that most of
these open launches are a stop the session declared in its transcript, which vigie
does not read — not a signal that never came. The figures here are re-measured with
that fix in place, by the same command.

Both describe the same residual and they do not read the same way. A launch is not
what an operator looks at; a row is. The per-session figure is the one that should
decide whether this price stays acceptable, and until #835 only the flattering one
was written down.

**What "open" does and does not prove.** A launch with no notification leaves its
session reading `working` until the session ends — but an open launch is not proof
of a lost signal, because the work may genuinely still be running. Since #834 that
is the normal case: a persistent `Monitor` emits nothing until something stops it.
Of the 8 sessions above, **4 had been inactive for more than two hours**, so for
those the work is certainly over and the status is certainly stale; for the other
four the transcript cannot say.

**Every figure here carries its corpus and its date, and none is ever carried
forward.** The numbers this section held before were 12 of 789, and they went out of
date within days — the corpus grew, and #834 widened what counts as background work.
A bare number with no provenance is what let the previous one survive being wrong.

**This figure replaces a much worse one, and the correction matters more than the
number.** It read *one in eight* — 161 of 1079 — and that was measured through a
parsing gap rather than off the wire: Claude Code delivers the notification two
ways, and vigie read only one of them, so a notification it never looked at
counted as a notification that was never sent (#818). Re-measured with the same
instrument over the same corpus, the sessions left latched fell from 17 of 87 to
4 of 87 once the second carrier was read.

The decision is untouched — no timer may decide what vigie cannot observe, and
that holds whether the residual is 16% or 1.5%. What was wrong was the *bill*:
nine tenths of the price this section asked the reader to accept was a defect, and
an accepted cost that is mostly a bug is the kind of sentence that stops anyone
from going to look.

Three things make that acceptable, and if any stopped being true this ADR would
have to be reopened:

1. **It is bounded by the session.** A process found gone reads `ended` before the
   tool refinements are consulted, so the wrong state dies with the session and
   never outlives it.
2. **It is the safe direction.** A session wrongly shown busy costs a missed
   opportunity; one wrongly shown at rest costs an interruption, which is what
   vigie exists to prevent.
3. **It does not reach attention.** `working` is not in the attention set, so a
   latched session never calls the operator and never affects the jump key.

### What this does not forbid

- **A liveness cap on a mechanism**, where the failure mode is unbounded and the
  cap is not evidence about the session. The subagent window (#344) is one: it
  bounds a `<task-notification>` format that may drift, on a signal — parent
  transcript activity — that a subagent's parent does produce. The distinction is
  whether the silence the cap keys on is *expected*. For a subagent it is not; for
  a background command it is the normal case.
- **Reading a duration to display it.** `SEEN`, the usage snapshot's age, the
  stale-report threshold: all describe, none conclude.
- **The read-time `stale`/`ended` split** (#284/#285), which is a threshold on
  *reports* — a fact about vigie's own observation, not an inference about the
  session. It says "nothing is watching this", which is exactly what it observed.

## Consequences

- A prompt no longer closes a backgrounded command (#810). It closes foreground
  calls and subagents, where the premise — *the session moved on* — holds.
- The residual above is accepted rather than mitigated, and `session-status.md` § 2
  says so where an operator would look. It is stated without a figure on purpose:
  this consequence outlives any measurement, and naming one here is how *one in
  eight* survived two corrections of the section it pointed at.
- A future proposal to cap something must show which of the two categories it is
  in: bounding a mechanism whose silence is unexpected, or concluding from
  silence that is expected. The second is refused here.

## Alternatives considered

**Cap the background command like a subagent.** Rejected above: the silence it
keys on is the normal state of the thing it measures.

**Cap on time since launch rather than since activity.** Same objection without
the disguise — a two-day build is a session working for two days, and no number
distinguishes it from a command that died.

**Accept the timer and narrow it to "very long".** Every threshold that is safe
for a build is useless for a lost notification, and every threshold that catches
the lost notification kills a build. There is no value that separates them,
because the two are indistinguishable in what vigie can see. That is the whole
finding.

## References

- [ADR-0012](0012-retire-the-stalled-status.md) — the first application
- [ADR-0014](0014-retire-remote-control-detection.md) — the second
- [design/session-status.md](../design/session-status.md) § 2 — where the
  background-command rule is specified
- [ADR-0005](0005-observe-only.md) — why a wrong state cannot be cleared by hand
