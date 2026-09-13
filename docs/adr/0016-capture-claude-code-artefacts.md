# ADR-0016: Fixtures describing Claude Code's artefacts are captured, not authored

## Status

Accepted (#817).

## Context

Everything vigie reports about a session is derived from artefacts Claude Code
writes for itself: the session registry under `~/.claude/sessions`, the
transcripts, and the hook payloads. That schema is private and undocumented, and
`sessionRecord` says so in as many words
(`internal/watch/registry.go:13-16`). It can change in any Claude Code release,
with no notice and no changelog.

**The replay harness is not the gap.** The suite already replays chronologies
over the real path, and [`code.md`](../code.md) already requires it for this bug
class — "sequence and interleaving bugs need a replay test over the real path":

- `internal/server/reconcile_timeline_test.go` replays hook events *and* watcher
  scans through the real report → store → view path on an injected clock,
  asserting the status at each tick (#203);
- `internal/watch/scan_replay_test.go` drives the real watcher over transcript
  fixtures, also on an injected clock;
- `internal/watch/run_replay_test.go` runs the whole watcher loop, every cadence
  read from the clock (#602).

**The gap is what they replay.** Every fixture's bytes are typed by hand.
`internal/watch/registry_test.go:26` is the whole problem in one line:
`{"sessionId":"s1","status":"waiting","waitingFor":"Allow Bash?"…}` is what
someone believed the registry looks like. A replay is exactly as true as its
input, and nothing in the suite can tell whether the belief still holds — or ever
did.

**The cost, measured.** #816: a session whose permission prompt the operator had
already answered went on reading `waiting` for as long as the tool ran. It was
reachable by the first harness above; nobody wrote the case, because writing it
required knowing that the registry says `busy` once the prompt is answered, and
nobody had recorded that. Confirming the defect took a human reproducing it on
their keyboard. Confirming the fix's premise took a recorder left running for
hours, and the first attempt measured its budget in wall-clock time, so an
overnight suspend consumed six hours of "observation" in which nothing was
observed.

**And the two halves do not meet.** `scan_replay_test.go` stops at the report the
watcher *would* send; `reconcile_timeline_test.go` starts from a report written by
hand. Nothing derives a report from a real artefact and then reconciles it, so a
defect spanning the join is invisible from both ends.

## Decision

**A fixture that describes a Claude Code artefact is recorded from a running
system. It is never authored, and never hand-edited afterwards.**

1. **Capture is passive, and redacted at the source.** A recorder samples the
   registry and the transcripts during ordinary use, writing a timestamped log of
   changes. Scenarios are *caught*, not performed: permission prompts,
   compactions, API errors and backgrounded commands all occur several times a day
   on their own, and the recorder's only job is to be running when they do.
   It **records its own gaps** — a suspended machine must never read as continuous
   observation, which is the error the first attempt made.

   **It never writes the sensitive bytes at all.** Not "capture then scrub": the
   recorder keeps only the fields the watcher reads, and substitutes a synthetic
   value for every free-text one *as it writes*. What a fixture needs is the
   structure and the order — which line types, which content blocks, which stop
   reason, which tool ids, at what times, and what the registry said at each
   moment. It never needs a working directory, a session title, a branch name or a
   line of prose. See § "Why not scrub" below.

2. **Existing harnesses consume captured content.** This step adds no harness. It
   replaces invented bytes with recorded bytes in the three replays above.

3. **A joined replay lives in `test/`.** A captured artefact produces a real
   report through the real watcher, and that report is reconciled by the real
   server. It cannot live in `internal/watch`: depguard forbids every client-side
   package from linking `internal/server`, as a CI failure rather than a review
   note (`.golangci.yml`, [ADR-0003](0003-split-client-and-daemon-binaries.md)).

4. **The vocabulary is guarded, twice.** A test fails loudly when the registry
   carries a value the code does not know: today `mapRegistryStatus`
   (`internal/watch/registry.go:87-98`) folds anything unrecognised into `idle`,
   silently, so a status renamed upstream would show the whole fleet at rest with
   the suite green. And a second test **allow-lists** what a recorded fixture may
   contain — synthetic ids, synthetic paths, known status words, ISO timestamps —
   failing on anything outside it.

## Why not scrub

This repository is public, and a raw capture carries working directories, session
titles, branch names and, in a transcript, whatever was pasted into the
conversation: keys, hostnames, a client's name. "Scrub before committing" is a step
a person performs, and there is direct evidence in this repository that such a step
does not hold. #589 replaced the operator's real machine and account names with
placeholders and closed as done; #631 found twelve more two weeks later, because the
sweep was a list of files rather than a check
(`test/docs/placeholders_test.go`).

That leak is still in the public history — the working tree was cleaned, the past
could not be. It is low harm (a machine name, a local account name, two home paths;
the commit metadata already carries more) and it is not worth rewriting history
for. What matters is its shape: **the content that escaped was hand-typed — small,
bounded, and known — and it escaped anyway, twice, on the same data.** Captured
content is none of those things. Applying the same discipline to it would fail in
the same way, at a far worse scale.

Hence the inversion, in both directions. Redaction moves from *before the commit*
to *at the moment of writing*, so the sensitive bytes never exist in a file and
there is nothing left to forget. And the guard moves from a **deny list** of names
already known to have leaked — which is the right shape for content a human typed —
to an **allow list** of what a recorded fixture may contain, which is the only shape
that holds for content the machine produced. Anything unanticipated fails by
default instead of passing by default.

**What is lost by substituting, and why it is nothing.** A synthetic path proves as
much as a real one: no test asserts whose directory it was. What the fixtures exist
to carry is that the registry said `busy` at a given moment while the transcript
stayed frozen — an order of events, which is exactly what was being invented
before.

**A chronology, never a snapshot.** A snapshot shows a handful of fields
consistent at one instant and says nothing about the order they moved in, which is
where #816 lived: the hook speaks, the registry flips a moment later, and the
transcript stays frozen throughout. A snapshot-driven fixture would have shown
three plausible values and proved nothing.

**Nothing sensitive is captured in the first place**, rather than removed
afterwards — the reasoning is in § "Why not scrub". The standing rule still binds
what survives substitution: this repository may name itself, and not what sits
next to it.

## Rationale

- **Keep authoring fixtures.** The status quo. It produced #816 and, worse, made
  it unconfirmable without a human at a keyboard.
- **Integration-test against a live Claude Code.** Authoritative and unrunnable:
  it needs an interactive session, a permission prompt performed on cue, and an
  API budget. It is the method this ADR exists to replace.
- **Keep manual reproduction as the regression method.** It does not run in CI,
  cannot be repeated on demand, and expires the day nobody remembers the gesture.
- **Generate fixtures from the schema.** There is no schema to generate from. That
  absence is the whole premise.

## Consequences

- **The recorder is a maintained tool, not a one-off script.** Content captured
  once and hand-edited afterwards is an authored fixture again — more convincing,
  and therefore worse, because it will be believed when it lies.
- **Coverage becomes a function of what has been observed.** A state no machine has
  produced has no fixture, and the suite must say so rather than imply a coverage
  it does not have.
- **Drift becomes loud.** Point 4 turns an upstream rename from a silent wrong
  answer into a failing build.
- **Nothing here reaches into a session.** The recorder reads the files the watcher
  already reads, and writes nowhere near them
  ([ADR-0005](0005-observe-only.md)).
- The steps are ordered and the order is part of the decision: capture before
  replacement, replacement before the join. Writing the join first means writing
  it against invented content, which is the thing being retired.

## References

- Issue #817; #816 is the defect that argued for it
- [`code.md`](../code.md) — the replay rule this ADR supplies the inputs for
- [ADR-0003](0003-split-client-and-daemon-binaries.md) — the client barrier that
  decides where the joined replay can live
- [ADR-0011](0011-derive-the-session-view-on-the-server.md) — shared fixtures for
  the Go/JS duplication: a different axis, unchanged by this
