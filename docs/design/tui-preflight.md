# TUI startup preflight

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
3. **Local watcher fresh** — added by [#359](https://github.com/haribo/claude-vigie/issues/359):
   only when this machine has vigie hooks installed, the preflight also requires a
   fresh local watcher heartbeat and a matching watcher version (see that issue).

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
