# Making `vigie call` discoverable — Design Specification

**Status:** Accepted (#391).

The session-raised call ([ADR-0010](../adr/0010-session-raised-operator-call.md))
only works if Claude actually runs `vigie call` when the operator asks to be told.
The operator writing *"tell me in vigie when you're done"* is not enough on its
own: nothing in a fresh session has ever taught Claude that the command exists.
This document records how it is taught, and what that costs.

---

## 1. Decision: a personal skill, installed by vigie

vigie installs an Agent Skill at:

```
~/.claude/skills/vigie-call/SKILL.md
```

A **personal** skill (as opposed to a project one) is active in every project with
no per-project setup, needs no `settings.json` entry — presence on disk is enough
— and does not require the workspace-trust acceptance that project skills do. Its
`description` is the matching signal Claude uses to decide when to load it, so
that line is the load-bearing part of this whole feature and is written to match
the way an operator actually phrases the request.

### Rejected alternatives

| Alternative | Why not |
| --- | --- |
| A snippet for each project's `CLAUDE.md` | Every user must copy it into every project. The feature would silently not work everywhere it was not pasted, which is the failure mode hardest to notice. |
| Documentation only | Lowest cost and lowest reach. It teaches the human, not the session — and it is the session that has to run the command. |

## 2. Lifecycle: owned by vigie, refreshed like the hooks

The skill directory is **owned entirely by vigie**. The file says so in its own
header, because Claude Code has no versioning or conflict detection for skills: a
refresh overwrites, and a local edit would be lost silently otherwise.

It is written by `vigie init` and `vigie hooks install`, refreshed by
`vigie watch` at startup beside the hooks refresh, and removed by
`vigie hooks uninstall`.

Refreshing at watcher startup **extends [ADR-0009](../adr/0009-watcher-managed-hooks.md)**
from one artefact to two, for the same reason it gave: an install predating a
release otherwise keeps a stale description forever, and the operator has no way
to know. Like the hooks refresh, it is best-effort — a failure is logged and never
stops the watch.

**Production leg only.** The dev leg (`VIGIE_CONFIG`) exists so a contributor can
run a second reporting leg *without touching production artefacts*, which is why
`vigie hooks uninstall` on the dev leg leaves production hooks alone. The skill is
not leg-scoped — there is one Claude Code configuration per machine — so only the
production leg writes or removes it.

## 3. The call is best-effort, and says so

A natural-language instruction is honoured non-deterministically, and the risk is
highest exactly where the feature matters: at the end of a long turn. When Claude
does not run the command, nothing is raised and the session reads exactly as it
does today — a silent but graceful degradation, never a wrong display.

This is stated in the skill itself and in the user-facing documentation rather
than left for the operator to discover. The deterministic alternative — Claude
*arms* the request and the `Stop` hook *posts* it — is recorded as deferred in
ADR-0010; if the forget rate proves high in practice, that is the fallback to
revisit.

## 4. Testing

- The skill is written to the expected path with parseable frontmatter carrying a
  non-empty `description` (the trigger signal) and mentions the exact command.
- `vigie hooks uninstall` removes it; a second uninstall is not an error.
- Install is idempotent: running it twice leaves one skill, byte-identical.
- The dev leg neither writes nor removes it.
