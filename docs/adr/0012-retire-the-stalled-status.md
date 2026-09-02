# ADR-0012: Retire the `stalled` status

## Status

Proposed.

## Context

`stalled` is one of the three statuses that call the operator. It claims a turn is
**parked on a hung tool** — a `tool_use` that never got its `tool_result`, with the
session gone quiet.

[session-status.md](../design/session-status.md) defends it as an exact signal:

> The pairing is exact (an id match), not a timeout guess.

That defence does not hold. The pairing proves a tool is **outstanding**. It says
nothing about whether it is *hung*. The hung-ness comes entirely from a timer —
45 s after a five-minute grace period — and a timer over a duration vigie cannot
interpret. A three-minute build and a three-minute hang are the same observation.

**Only the operator holds the missing half.** They asked for the command. They
know an end-to-end suite runs for an hour and a `git status` does not. vigie
observes a duration; the judgement is not its to make.

### What was measured

Sampling Claude Code's registry every 2 s for two minutes on a live session,
across a long foreground command (#661): **21 samples `busy`, then 39 `shell`**.
The registry does not stay `busy` for a command's duration.

`shell` mapped to `idle`, and `idle` is the base the tool pairing acts on. So an
hour-long test suite was reported `stalled`, which is exactly the false positive
§ 2 of the design says the five-minute window exists to prevent. The window is
real; it never governed the path a current Claude Code is on.

### What already changed

[#661](https://github.com/haribo/claude-vigie/issues/661) made `shell` with an
unanswered `tool_use` read `working`. That removed the false positive on this
path as a side effect — the pairing acts on an `idle` base, and the base is no
longer `idle`.

It did not remove the claim. `stalled` is still in the vocabulary, still in the
attention set, still reachable when the registry reports `idle` with a tool
outstanding, and still described in the design as a reliable signal.

### What the operator already sees

The transcript freezes on the `tool_use` line while a command runs, so the SEEN
column counts from exactly that moment. With `working` and the tool's name on the
row, the table already reads:

```
acme-api   ● working   12m   Bash: run the e2e suite
```

Twelve minutes on an e2e suite is normal; twelve minutes on a `git status` is not.
The row carries what the operator needs to tell those apart, and vigie does not.

## Decision

**Remove `stalled` from the status vocabulary.** A session waiting on a command
is `working`. How long it has been waiting is on the row, and the operator judges.

- The vocabulary goes from nine statuses to eight. `internal/status` is the
  source, and the daemon refuses a report carrying anything else.
- The attention set — the statuses that call the operator — becomes `waiting` and
  `error`, plus a call the session raised for itself
  ([ADR-0010](0010-session-raised-operator-call.md)).
- The tool pairing stays where it earns its place: closing an orphan on a real
  prompt (#483, #662) and keeping a background task `working`. What goes is the
  arm that turns silence into a verdict.
- `session-status.md` § 2 loses its `stalled` derivation and its measurement, and
  says instead what the row shows and who reads it.

This does not narrow [ADR-0008](0008-compacting-status.md), which added
`compacting` as a *refinement* of `working` rather than an authoritative status.
The enum being closed is what makes removing a member an ADR rather than a commit.

## Consequences

- **A live session whose command will never return no longer calls anyone.** It
  reads `working`, with a duration that grows. This is the real loss, and it is
  smaller than it looks: a session that *died* mid-tool leaves no process, so it
  reads `ended` already ([ADR-0006](0006-session-presence-via-proc.md)). What
  remains is a live Claude waiting forever on a command — where `running for 4 h`
  is self-evident to a human and was not to the heuristic.
- **No long command is ever announced as a fault again.** The signal that fired on
  builds, test suites and long searches is gone, and with it the training to
  ignore the notification channel `waiting` and `error` share.
- **Three clients, one vocabulary.** 43 files carry the word. The colour palette
  loses a member (`stalled` had its own hue in both themes), the GNOME indicator's
  menu grouping loses a section, `test/fixtures/status-vocabulary.json` and
  `status-colors.json` shrink, and the shared fixtures keep the three suites
  honest through the change.
- **Stored history keeps the word.** Rows and events written before this release
  may carry `stalled`. Clients must not break on a status they no longer know —
  the vocabulary's "unrecognised degrades to idle" rule already covers it, and it
  should be tested rather than assumed.

## Alternatives considered

**Restore the five-minute window on the registry path.** Makes the two paths
agree and is what the design already documents. Rejected: it keeps a verdict vigie
has no grounds for. A ten-minute build would still be announced as a fault — the
threshold moves, the category error does not.

**Make the threshold configurable.** Rejected for the same reason, and it asks
each operator to guess a number that depends on the command rather than the fleet.

**Keep `stalled` but drop it from the attention set.** A status that never calls
anyone is a label, and a label that claims "hung" while meaning "still running" is
worse than no label.
