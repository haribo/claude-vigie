# Daily token rollup — Design Specification

**Status:** Accepted (#432, #433).

Source of truth for how output tokens reach `stats_daily`, and for the rule that
keeps that table's history trustworthy. Per-session usage is a different thing: it
lives on the session row and is simply the last figure reported.

---

## 1. What makes this table special

`stats_daily` is **never pruned** — that is its purpose, to retain history beyond
the session-retention window. Two consequences follow, and together they are why
this document exists:

- Rows are **accumulated, never recomputed.** The source they came from (session
  rows, events, samples) is pruned; nothing can rebuild a day.
- A wrong value is therefore **permanent**, and it poisons every aggregate built
  on top of it — every period either client offers. The two do not offer the same
  ones: the terminal buckets history (twelve ISO weeks, stacked by model) and the
  dashboard sums a rolling window (the last 7 days, as one figure). They shared
  the word `Week` until #666, which is why this line used to name the periods as
  if there were one set.

A rollup writing into such a table must be conservative by construction: it may
under-count a day, never double-count one.

## 2. The defect

The rollup accumulated the growth of a counter it does not own:

```go
delta := sess.Usage.OutputTokens - old.Usage.OutputTokens
if delta <= 0 { return }
```

`old` is the session row as last stored. Whenever that value regresses, the next
report's delta is the session's **entire lifetime total**, added again — and
nothing bounded it or flagged it.

Observed in production: `2026-08-12 / claude-opus-4-8 / output_tokens =
61051295773`. Sixty-one billion, where every other (day, model) pair over the same
two weeks sits between 3.5k and 4.3M. The session's own figure was correct:
2 713 408. The ratio is ≈ 22 500.

### 2.1 Two reproductions, both from observed data

**One session, two transcript files.** Claude Code stores transcripts under
`~/.claude/projects/<encoded-cwd>/<sessionId>.jsonl`. A session whose working
directory changes is written under two directories with the *same* session id. On
the machine that produced the figure above:

| file | last write | output tokens | model |
|---|---|---|---|
| `…-note-git-note/3c52e3ce….jsonl` (1.3 kB) | 2026-08-12 11:14 | 0 | *(none)* |
| `…-plain-note-git-plain-note/3c52e3ce….jsonl` (56.7 MB) | 2026-08-14 14:04 | 2 713 408 | `claude-opus-4-8` |

The scanner globs `*/*.jsonl` and emits **one report per file**, so each scan sent
two reports for one session: the real total, then zero. The drop was skipped by
`delta <= 0` but still **overwrote** the stored figure, so the next scan's rise
looked like 2.7M of fresh output. At a 2 s scan interval, from 11:14 to midnight,
that is ≈ 22 500 additions — the observed ratio.

The smaller file holds nine metadata lines and not one exchange. Seven further
such files exist on the same machine with no conversation anywhere; they are
reported as sessions today, which is a separate display defect.

**Resuming a session older than the retention window.** Reproduced against the
server, no transcript involved:

```
after the initial session         : 1 000 000
sessions removed by retention     : 1
after the resume                  : 2 000 000   ← the transcript never changed
```

A session goes quiet for longer than `--max-age`, so the watcher stops reporting
it; `--session-retention` then deletes the row while the transcript stays on disk.
Resuming it makes the watcher report it again, the daemon does not recognise it,
and the whole lifetime total lands in today's bucket. Both windows default to 24 h,
and resuming an old session is ordinary use.

## 3. What can and cannot make a total regress

Verified against 148 real transcripts:

- **Compaction does not reset it.** The boundary is appended to the same file and
  earlier turns remain, so the cumulative figure keeps climbing — measured across
  four successive compactions in one session: 846 610 → 1 619 262 → 2 372 769 →
  3 084 938 → 3 703 974.
- **`/clear` does not reset it either.** It starts a *new* session id, and a
  transcript's filename is always its session id (148 of 148). The new session has
  its own counter; the old one keeps its total.

So within one session a legitimate total only ever grows. Every regression seen so
far comes from vigie losing track of what it had already counted — not from the
session.

## 4. Decision

**Count against a per-session high-water mark, held outside the session lifecycle.**

For each session, `stats_daily` records how many output tokens have already been
counted. A report adds only what exceeds that mark, and raises it:

```
counted = max(counted, reported_total)   // add the difference, if any
```

- A regression adds nothing, whatever caused it.
- A return to a previously seen figure adds nothing — the tokens were counted once.
- Growth beyond the mark is added exactly once.

The mark lives in its own table, **not on the session row**. Storing it on the row
would delete it with the row, which is precisely the case in § 2.1's second
reproduction.

The mark is never pruned, for the same reason `stats_daily` is not: it is what
makes that table's history correct. Its cost is one row per session ever seen —
a session id and an integer.

**The mark and the daily bucket move together, in one transaction.** The mark
means *this much is already in `stats_daily`*, and raising it in a call of its own
let that become true ahead of the fact: the mark advanced, the daily write failed,
and because the table is never recomputed the growth was gone for good — silently,
with nothing recording which day `vigied stats-repair` should be pointed at. Rare,
since the insert fails only on a write error, and permanent when it happens. In
one transaction a failure leaves the mark where it was, and the next report counts
the same growth again (#669).

### 4.1 Also: one report per session per scan

The scanner keeps a single report per session id, the one carrying the largest
total. Two files claiming one session means one of them is no longer being written
to; the larger total is the live one.

This removes the cause; § 4's mark removes the *class* of failure. Both are worth
having — the mark is what holds when the next unforeseen regression appears, and
§ 2.1's second reproduction shows the causes are not all in the scanner.

## 5. Rejected: a plausibility bound

Refusing any delta above a threshold was rejected. It requires a number nobody can
justify: too low and it silently discards real output from a busy session, too high
and it passes exactly the multi-million re-count seen here. It also treats the
symptom while leaving the rollup's assumption — that the session counter never
regresses — false and unstated.

The high-water mark needs no threshold. It is exact.

## 6. The model a bucket is keyed by

Both rollups key on the session's model, so what counts as a model decides which
bucket real work lands in.

**A marker is not a model.** Claude Code writes bracketed markers in an assistant
line's `model` field for lines it generated itself rather than received from the
API. `<synthetic>` is the one observed. Unfiltered, it became the session's model
until the next real turn, and every token produced meanwhile was attributed to it —
a production day held `<synthetic> / output_tokens = 12879`, real output taken from
the real model. The parser now keeps the last *real* model instead (#433).

The test is the bracket, not the API-error flag. Of nine synthetic lines found
across one machine's transcripts, only five carried `isApiErrorMessage: true`; the
other four are ordinary lines ("No response requested."), so an error-flag check
would have missed them.

**An unknown model stays its own bucket, `""`.** A session can report before any
assistant line exists — nothing has named a model yet. That bucket is honest: it
means "not yet known", and it carries real status time.

It cannot distort token figures. Output tokens only come from assistant lines, and
those carry a model, so an unknown model implies no tokens — verified across 314
real transcripts: 18 have no model, and **none of them has a single output token**.
Skipping the rollup instead would discard the status time for no gain.

## 7. Repairing a poisoned row

Since a day cannot be recomputed, correction is a deliberate operator act:

```
vigied stats-repair -db <path> -day 2026-08-12 -model claude-opus-4-8 -output-tokens N
```

It sets one `(day, model)` row's output tokens to a stated value, prints what it
replaced, and touches nothing else. There is no automatic detection: a large day is
not, by itself, wrong.

## 8. Consequences

- A day can now be **under**-counted — a session whose total regresses stops
  contributing until it climbs past its previous peak. That is the deliberate side
  of "conservative by construction": an under-count is a gap, a double-count is a
  permanent lie.
- The mark table grows with the number of sessions ever seen and is never pruned.
- `stats_daily` gains no schema change, so existing history is untouched, correct
  rows and poisoned ones alike.
