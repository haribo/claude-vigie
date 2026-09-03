# ADR-0013: Only Claude Code may report a session

## Status

Accepted (#709).

## Context

The sessions table grew rows the operator could not account for: named by an
eight-character hash instead of a title, in a project directory they recognise,
with no terminal behind them. Counting `claude` processes stopped matching
counting rows — one extra terminal produced five extra rows.

They were real sessions of a different CLI. That CLI reads
`~/.claude/settings.json` and runs the hooks it finds there — documented, on by
default — so it calls `vigie report` exactly as Claude Code does, and nothing in
vigie can tell the two apart.

**This is not a permissions failure.** The caller is a trusted machine holding a
valid token, which is what the threat model promises
([deployment.md](../deployment.md)). The gap is that nothing establishes *which
program* is reporting.

### Why partial support is not support

vigie's session model is Claude Code's, in three places at once:

- **The name** is the title read from the transcript under `~/.claude/projects/`.
  A foreign session has none, so `nameView` falls back to the short id — the hash
  the operator sees.
- **Liveness** comes from a process mapping captured at `SessionStart`, and
  `presence.ResolveClaude` walks the ancestor chain for a process named `claude`.
  Under another CLI it finds none and saves nothing, so the row can never read
  `ended` from presence — only age out by retention.
- **A subagent is deliberately not a session** ([#344](https://github.com/haribo/claude-vigie/issues/344)):
  it refines the parent's status from the parent transcript. That holds only
  because Claude Code gives a subagent the parent's `session_id`. A CLI that gives
  each subagent its own id produces one row per subagent. There was never a filter
  here — the guard was Claude Code's numbering, not ours.

So a foreign harness does not get a reduced view. It gets wrong rows, and the
fleet count stops meaning anything — which is the one question the board exists to
answer.

### What was measured

Claude Code hands every process it spawns an environment another CLI does not
share. A throwaway `PostToolUse` hook dumping its own environment and stdin
confirmed that a **hook** process — not only a tool process — carries it:

```
env   CLAUDE_CODE_SESSION_ID = a78b5daf-44bf-4e28-93a8-c6ad5e5a62d7
stdin session_id             = a78b5daf-44bf-4e28-93a8-c6ad5e5a62d7   → equal
CLAUDECODE=1   CLAUDE_CODE_ENTRYPOINT=cli   CLAUDE_PID=157048
```

The foreign CLI sets none of these. Its own runner injects `GROK_*` variables and
one Claude-named one, `CLAUDE_PROJECT_DIR`, for compatibility.

## Decision

**vigie supervises Claude Code, and reports come from Claude Code.**

**1. `report` refuses to post unless `CLAUDE_CODE_SESSION_ID` is set and equal to
the payload's `session_id`.** Presence of the variable proves a Claude-shaped
environment; equality proves the caller is reporting its own session rather than
relaying someone else's.

The check belongs on the client, in `internal/report`, before the post. The server
cannot help: it receives JSON, and any `harness` field in it would be
self-declared by the party we are trying to identify.

**The absent case refuses.** A foreign CLI sets nothing, so "absent → allow" is no
guard at all.

**2. The refusal is observable before it is strict.** A hook always exits 0, so a
silent refusal would mean that the day Claude Code renames or drops the variable,
vigie loses the whole fleet and says nothing —
[ADR-0006](0006-session-presence-via-proc.md)'s own failure shape one layer up,
and the one #663 was about. A refused report must leave a trace the TUI preflight
can surface, next to the unreachable-daemon and version-drift checks it already
runs.

**3. Presence reads `CLAUDE_PID` when it is there.** The same environment carries
the Claude process directly. `ResolveClaude` climbs up to twenty ancestors matching
a process name against `claude`, working around the kernel's 15-byte truncation, to
recover a number the environment already holds. Reading it removes a walk that a
wrapper or a shim can break, and it is a second discriminator with a property the
first lacks: a path like `CLAUDE_PROJECT_DIR` is something another harness has
reason to alias, a pid is not. The ancestor walk stays as the fallback.

## Consequences

- **A foreign harness stops appearing.** Not degraded — absent. That is the point:
  a row vigie cannot name, cannot age out and cannot count correctly is worse than
  no row.
- **vigie's scope is now written down.** It was "anyone" by silence rather than by
  choice. Anyone wanting to supervise another harness has a documented decision to
  argue against rather than a gap to fall through.
- **A rename breaks the fleet, loudly.** This keys on a convention Anthropic does
  not owe us. Decision 2 is what makes that survivable, and it is not optional —
  without it this ADR trades wrong rows for no rows, silently.
- **Existing foreign rows stay** until retention removes them. Nothing here
  deletes stored sessions; the rule governs what is accepted next.

## Alternatives considered

**Supervise every harness.** Carry a declared `harness` field, and make naming,
presence and subagent handling per-harness. Rejected as a much larger decision
than the defect calls for: it changes what the product is, and every one of the
three assumptions above would need a per-harness answer written and tested. It
remains available — this ADR is what it would have to supersede.

**Filter on the server.** Rejected: the server sees only JSON. A `harness` field
would be declared by the caller we are trying to identify, and the check would be
worth exactly the honesty of whoever sends it.

**Key on `CLAUDECODE=1` alone.** Simpler, and it proves the environment without
proving the session. A relay — a wrapper posting for sessions that are not its own
— would pass. Equality with the payload costs nothing more and closes that.

**Do nothing and document it.** Rejected: the rows are not a cosmetic problem. They
break the fleet count, which is the board's one job.
