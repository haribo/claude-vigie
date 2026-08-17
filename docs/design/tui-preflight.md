# TUI startup preflight — Design Specification

**Status:** Accepted (#357).

`vigie tui` runs a **preflight** before it enters the terminal's alt-screen. A
down or mismatched daemon must fail loudly on the normal screen — never degrade
silently behind a full-screen UI the operator then has to quit to read the error
(#357).

## Checks (in order)

1. **Server reachable and token valid.** The same probe as `vigie init`
   (`GET /api/sessions`): a transport error, `401` (bad token), or `404`
   (not a vigie server — wrong port, or `vigied` not running there) fails here.
2. **Version match.** Fetch `GET /api/version` and compare the daemon's build to
   this TUI's (`internal/version`):
   - **strict equality** of the version string when both sides are a real
     release (e.g. `0.3.0` vs `0.3.0`);
   - when **either side is `dev`**, compare the **commit** instead — a
     `"dev" == "dev"` string match across two different builds is a false pass.
3. **Local watcher fresh** (#359). Only when this machine has vigie hooks
   installed (a `report --event=` marker for this leg in `settings.json`), the
   preflight also requires, from `GET /api/watcher`, a **fresh** local heartbeat
   (the same 15 s stale threshold as the TUI) and a watcher **version** that
   matches this TUI (same `dev`-by-commit rule). A machine with no local hooks is
   a pure observer and starts normally — no watcher required. Because the watcher
   owns the hooks lifecycle ([ADR-0009](../adr/0009-watcher-managed-hooks.md)),
   watcher version == hooks version, so this one check also proves the local
   hooks are current.

   **Stale heartbeat is not proof of a dead watcher** (#371). The heartbeat is a
   server round-trip: a watcher that is genuinely running but whose report has
   not landed — a just-restarted watcher, or an unreachable/erroring server —
   looks identical to one that is down. So when the heartbeat is stale, the
   preflight first cross-checks a **local** liveness signal: is another instance
   of *this binary* running the `watch` subcommand on this machine (a `/proc`
   scan, Linux only)? This decides the remediation:
   - **no local watcher process** → "watcher not running, start it" (the true
     down case);
   - **a local watcher is running** → the watcher is up but the server has no
     recent heartbeat from it — the fault is the server or a just-started
     watcher, so the message points at `vigied`/connectivity and says to retry,
     **not** to restart the watcher.

   The `/proc` scan is best-effort: off Linux, or if it cannot read `/proc`, the
   preflight falls back to the plain "watcher not running" message rather than
   guess.

## On failure

- **No alt-screen.** The failure is printed to stderr on the normal terminal.
- The message states **both versions** (or the concrete connection error) and the
  **exact remediation** (upgrade the older side, fix the token/URL, restart the
  watcher).
- **Exit 1.**
- **No skip/force flag.** Strict means strict: a fleet-consistency tool that lets
  you bypass its own consistency check is not one. Fix the drift, or point at a
  matching daemon.

## Rationale

Version drift between a client and the daemon it talks to fails in confusing ways
(a client too old for a newer API shape). Because the watcher owns the hooks
lifecycle ([ADR-0009](../adr/0009-watcher-managed-hooks.md)), watcher version ==
hooks version, so the #359 watcher check also proves the local hooks are current
— one check, both guarantees.
