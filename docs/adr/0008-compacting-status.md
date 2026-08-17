# ADR-0008: Surface a `compacting` status via a PreCompact marker

## Status

Accepted.

## Context

When Claude Code compacts a session's context it goes silent for **87 s to 168 s**
(observed range over 38 real compactions) while it summarizes. Throughout, Claude
Code's session registry reports `busy`, so the watcher shows `working` — correct
but opaque: the operator sees ~2 minutes of apparent work with no output, then
watches the context gauge ([#279](https://github.com/haribo/claude-vigie/issues/279))
collapse for no visible reason. Vigie exists to say what a session is doing;
here it says the wrong-feeling thing.

What Claude Code exposes (verified against `claude` 2.1.220):

- **Nothing is written when compaction starts.** The transcript simply stops.
- **The end is marked** by a `{"type":"system","subtype":"compact_boundary",…}`
  line, recoverable after the fact — useless as a *live* signal.
- **The registry status enum is closed** (`busy`/`shell`/`idle`/`waiting`); a
  `compacting` value exists in the binary but is only an `sdk_status` event to
  SDK clients, never written to the registry file.
- **`PreCompact` is the only live start signal.** It fires before compaction with
  a `manual`/`auto` matcher and blocks only if the hook explicitly asks to.

So a live status requires a hook. This is the **seventh** hook we install on
every machine (see the hook table in [`architecture.md`](../architecture.md)) — a
deliberate widening of the install surface. It is judged worth it: installation
is one entry in the client's default hook list (the watcher writes them all at
startup, [ADR-0009](0009-watcher-managed-hooks.md)),
the CPU cost is a single fast `exit 0` per compaction (rare), and the value is
on-mission — the sibling of [#344](https://github.com/haribo/claude-vigie/issues/344)
(subagents refine `idle`→`working`). It fills no critical gap; it makes an
opaque two minutes legible.

## Decision

**Model `compacting` as a client-side refinement of `working`, opened by a
`PreCompact` marker and closed by the transcript boundary or a timeout.**

- **Refinement, not a new authoritative status.** Exactly the `thinking`
  precedent: derived in the watcher after the registry (which stays
  authoritative), applied right after `withThinking` — `working`/`thinking`
  become `compacting`. It is never sourced from the registry or an API field.
- **Open (marker).** A new `PreCompact` hook writes
  `~/.local/state/vigie/compacting/<sessionId>.json = {"started","trigger"}`,
  mirroring the presence mechanism ([ADR-0006](0006-session-presence-via-proc.md)):
  the path is derived from `HOME` (not `XDG_STATE_HOME`) so the hook's and the
  watcher's environments resolve to the same directory. The hook exits 0 and
  never sets `blockedBy`, so it can never interfere with the compaction it
  observes ([ADR-0005](0005-observe-only.md)).
- **Close (boundary).** The watcher parses the last `compact_boundary` timestamp
  from the transcript. A boundary at or after the marker's `started` means the
  compaction finished; the refinement clears and the marker is swept.
- **Expire (timeout).** A `compactWindow` safety cap (5 min — comfortably past the
  observed 168 s max) clears a marker whose boundary never arrived, so an
  interrupted compaction never pins the status. A killed session is already
  `ended` (registry-dead wins before the refinement runs), so only the rare
  alive-but-aborted case relies on this.
- **Server treats it as active work.** `compacting` clears a hook-posted
  `waiting` like `working`/`thinking` — it *is* working, just a named kind. Like
  `thinking`, it is a display refinement: the time-by-status rollups bucket only
  the base `working`/`waiting`/`idle`, so a refinement is not separately counted
  (consistent with the existing `thinking` behavior, not a regression).

  > **Amended by #508.** The first sentence no longer holds: `compacting` does
  > **not** clear a hook-posted `waiting`. The reason has nothing to do with
  > compaction. To the watcher, "a tool is running" and "a permission prompt is
  > blocking" are the same thing — a turn stopped on `tool_use` with a frozen
  > transcript — so *any* status it infers from that silence may not retract a
  > `waiting` a hook established, until the transcript actually moves past when
  > that `waiting` was posted. The rule was an allow list naming
  > `working`/`thinking`/`compacting`; `stalled` fell straight through it when
  > #256 added that status, and a permission prompt read as a hung tool for the
  > rest of the session. It is a **deny** list now — `error` and `ended`, the two
  > the watcher observes positively — so a status added later is held by default,
  > which is the safe direction: a late release costs less than naming the wrong
  > cause. See [session-status.md](../design/session-status.md) § 3 and
  > `watcherObserves` in `internal/server/report.go`.
  >
  > The rest of this decision stands: the `PreCompact` marker, the
  > `compact_boundary` close, the timeout cap, the rollup behaviour, and that
  > `compacting` is not an attention status.
- **Not an attention status.** Excluded from `attentionStatuses`: no desktop
  notification, no slot in the jump-to-next-waiting queue. Compaction needs
  nobody.

## Consequences

- **A seventh hook.** New installs get it for free (one entry in the default hook
  list); existing machines pick it up when their watcher next starts, or with
  `vigie hooks install` on a machine that runs none — `vigie init` no longer
  touches the hooks ([ADR-0009](0009-watcher-managed-hooks.md), #415). This is the
  price paid for a live start signal — there is no hook-free alternative.
- **Graceful degradation.** A machine without the hook installed gets no live
  `compacting`, only the after-the-fact boundary — the same hooks-free posture as
  the rest of the pipeline. Nothing breaks; the status just stays `working`.
- **A new local-state directory** (`~/.local/state/vigie/compacting/`), swept by
  the watcher when a boundary lands or the window expires.
- **Bounded stuck-status risk.** If both the boundary and the expiry were to
  misfire, a live session could read `compacting` for up to `compactWindow`; the
  cap makes it self-heal, exactly as `agentWindow` does for subagents (#344).
- **Observe-only holds.** The hook reports and exits 0; reading a marker file and
  the transcript is pure observation ([ADR-0005](0005-observe-only.md)).
