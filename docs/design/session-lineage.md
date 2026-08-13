# Session lineage — Design Specification

**Status:** Accepted.

Source of truth for how vigie handles a `/clear` (and any in-process session
switch): what the operator sees when a running session is replaced by a fresh
one on the **same process**. User-observable behavior, not the code. Like
everything in vigie, it is **detected**, never operator-set
([ADR-0005](../adr/0005-observe-only.md)).

Status semantics live in [session-status.md](session-status.md); this doc covers
the data (MODEL, EFFORT, CTX) and the ghost row a `/clear` would otherwise leave.

---

## 1. What `/clear` does, and why it looked broken

`/clear` does not restart Claude Code — the **process stays**, its PID unchanged.
It mints a **new session id**, writes a **fresh transcript** (carrying only the
custom title, mode, and the command echo — **no assistant line yet**), and
rewrites the process's registry entry (`~/.claude/sessions/<pid>.json`) to the
new id. The old transcript stays on disk, still fresh.

vigie reads MODEL, EFFORT, and CTX **only from assistant lines** of the
transcript. A just-cleared session has none, so all three read empty until the
first turn completes. Because `/clear` keeps the custom title, the row looks like
*the same session* losing its values (#367). Two more consequences fall out:

- the pre-clear session (old id, no longer in the registry, transcript still
  fresh, process still alive) keeps being scanned — a **ghost row**;
- CTX shows `-` (unknown) when the truthful answer is **0%** (known-empty).

The fix is not to read the fresh transcript differently — there is nothing to
read. It is to recognise the **lineage**: the new session inherits from the one
the same process was just running.

---

## 2. The link is the process, not the session id

Nothing records a `previousSessionId`. The registry holds only the process's
**current** session; the fresh transcript references the old one only through
`parentUuid` chains (by UUID, not id). The one cheap, reliable link is the
**process identity** — the pair `{PID, procStart}` (the registry's `procStart`
is the same `/proc` start-time vigie already uses to tell a live process from a
reused PID). Two session ids that share a process identity are the same lineage.

This is entirely read-only: vigie observes the registry and the presence
mappings it already reads, and derives the link. No new signal, no write.

---

## 3. What the operator sees

**MODEL and EFFORT carry over.** A `/clear`'d session shows the process's last
known model and effort immediately, instead of `-` until the first turn. The
watcher remembers, per process identity, the model and effort of the live
session it is watching; when that process's session id changes and the new
transcript has none yet, the new row inherits them. The inheritance is replaced
the moment the fresh session writes its own first assistant line — so a model or
effort the operator changed *at* the clear corrects itself one turn later.

Carry-over is **per process identity**, never global: a different window, a
different machine, or a reused PID (different `procStart`) never donates its
values. If the watcher restarts across the clear it has nothing to remember, and
the row falls back to `-` until the first turn — a transient miss, not a wrong
value.

**CTX reads 0%, not `-`.** A just-cleared session's context is **known-empty**,
not unknown. The two are distinct: *known-empty* is a live session whose
transcript vigie parsed and found no usage yet (0% is the honest reading);
*unknown* is a session vigie has no context reading for at all (a hooks-only
session before its first `Stop`), which still shows `-`. CTX is **not** carried
over — the fresh context genuinely starts at zero.

**No ghost row.** The pre-clear session is **superseded**: its process now backs
a newer session id. vigie reports it `ended` (see
[session-status.md](session-status.md) §2), so it drops off the default view
instead of lingering as a duplicate `idle` row until it ages out.

---

## 4. Why known-empty needs its own signal

`0` cannot mean both "unknown" and "known-empty" — the whole chain (watcher →
API → store → TUI) historically read `0` as unknown and rendered `-`. So context
carries an explicit **known** flag alongside the count: absent = unknown (keep
the last known value on merge, render `-`); present = known (overwrite on merge,
render the percentage, including `0%`). The watcher parses the transcript on
every scan, so it always reports a *known* context; a value only stays unknown
for a session no watcher has parsed.

---

## Appendix — doc conventions

`docs/design/` = the *what* (user-observable); `docs/adr/` = decisions with
rationale; code = the *how*. Docs never paraphrase code. Adding/modifying design
docs requires explicit consent — propose first.
