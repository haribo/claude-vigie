# Changelog

All notable changes to claude-vigie are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project aims to
follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Each GitHub Release's notes are a mirror of that version's section below — this
file is the single source of truth, not a second narrative.

## [Unreleased]

### Changed

- `vigie init` now writes the config and nothing else. The reporting hooks and the
  call skill had **three** writers — `init`, `vigie hooks install` and `vigie
  watch` — and only the watcher's copy self-heals, which is the whole point of
  [ADR-0009](docs/adr/0009-watcher-managed-hooks.md). They now have one owner: the
  watcher installs them at startup and keeps them matching the running binary.
  `init` ends by saying what is left to do — start the watcher, **or restart it**
  if one is already running, since it reads the config only at startup. This also
  removes a trap: `init` used to rewrite the *production* hooks even when
  `VIGIE_CONFIG` pointed at a dev leg. A machine that runs no watcher still wires
  itself with `vigie hooks install`; `vigie init --uninstall` keeps working but is
  deprecated in favour of `vigie hooks uninstall` (#415).

### Fixed

- Desktop notifications could be **impossible with nothing saying so**. The TUI
  assumed it had focus until the terminal said otherwise, so a terminal or
  multiplexer that never reports focus events suppressed every notification
  forever — correct settings, working desktop, and nothing ever arriving. Focus is
  now three-valued and *not knowing* no longer counts as "you are watching": a
  notification while you are already looking is a small annoyance, never receiving
  one is a broken feature. The Settings tab also now says **why** notifications
  cannot be delivered — `on — notify-send not installed`, `on — no graphical
  session` — instead of showing a cheerful `on` on a machine where nothing can
  work (#411).

### Changed

- `vigie init` now **asks** for the server URL and the token instead of requiring
  them as flags, and reads the token **without echo** — a flag put the shared
  secret into the shell history of every machine, permanently, which is the same
  reason it has no place in a systemd unit. The values are resolved in order:
  `--server`/`--token` if given, else `VIGIE_SERVER`/`VIGIE_TOKEN` from the
  environment, else a prompt. A non-interactive run that supplied neither fails
  with a message naming all three ways rather than hanging on a question nobody
  can answer, so containers and provisioning keep working (#407).

## [0.5.0] - 2026-08-13

### Added

- `vigie` now installs a personal Agent Skill (`~/.claude/skills/vigie-call/`) so
  Claude knows the `vigie call` command exists without any per-project setup — the
  whole feature rests on the command actually being run when you ask to be told.
  It is written by `vigie init` and `vigie hooks install`, refreshed by
  `vigie watch` at startup so an install predating a release cannot keep a stale
  description, and removed by `vigie hooks uninstall`. The production leg alone
  owns it: a dev leg touches no production artefact. The skill states plainly that
  the call is **best-effort** — if Claude does not run it, nothing is raised and
  the session reads exactly as it does today
  ([design](docs/design/call-discoverability.md), #391).
- The web dashboard surfaces a session's call with the same grammar as the TUI —
  the marker lives in the status, never in the Detail cell. The dot inside the
  status pill **pulses** (a soft CSS fade, where the terminal is limited to two
  hard states), the pill keeps its status color and label because a calling
  session is still `idle`, the row takes the existing attention left-border plus a
  faint tint of the same color, and the call message fills the Detail cell in that
  color. No new color: everything derives from `--st`. `prefers-reduced-motion:
  reduce` stops the pulse, and a call counter leads the summary strip when
  non-zero ([ADR-0010](docs/adr/0010-session-raised-operator-call.md), #390).
- The TUI now surfaces a session's call by **motion**: its status dot blinks in
  its own status color, at 1 Hz (inside WCAG 2.3.1's three-flashes-per-second
  ceiling), and the call message takes the `DETAIL` cell in that same color. No new
  glyph, color or column — the dot is the one element in a row that can be
  animated without destroying information, since the status word stays readable
  beside it. A `● call N` counter leads the summary strip when non-zero, a raised
  call reuses the desktop notification, and it jumps ahead of the inferred
  attention states in the `n` queue — a call is explicit where `waiting` is a
  deduction. The animation tick exists only while something is actually calling;
  the ambient poll stays at 5 s. Two preferences in `tui.toml`: `blink = false`
  stops the animation, and `call_marker` changes the glyph for fonts that lack it
  — a marker wider than one terminal cell is rejected rather than allowed to shift
  every column to its right ([ADR-0010](docs/adr/0010-session-raised-operator-call.md), #389).
- A session can now raise an explicit **call** for the operator: ask for it in
  plain language ("when you're finished, tell me in vigie") and Claude runs
  `vigie call "backfill done — 12k rows"` at the end of its turn. vigie surfaces
  the call until work resumes in that session. The call is set **and cleared by
  the session** (`UserPromptSubmit`, `SessionEnd`), so no action on vigie is ever
  required — it is an observed signal like status, not operator handling state,
  which is what keeps it on the right side of
  [ADR-0007](docs/adr/0007-read-only-to-operator.md)
  ([ADR-0010](docs/adr/0010-session-raised-operator-call.md), #388). It is
  orthogonal to status — a calling session keeps whatever status it has — and the
  message is optional. Like the hooks it is fire-and-forget: it can never fail a
  session. Rendering lands separately (TUI #389, web #390).
- The sessions table now scrolls within a vertical viewport instead of spilling
  off the bottom of the terminal. It tracks the terminal height (previously only
  width was honored), keeps the tab bar, summary, column header, and usage/footer
  pinned, and scrolls only the row band — continuous, cursor-driven, htop/k9s
  style, with a 2-row look-ahead margin and a `rows a–b / n` indicator shown only
  when the list overflows. Grouped views keep the current group's header pinned;
  the detail panel scrolls the same way ([design](docs/design/tui-viewport.md),
  #378).

### Changed

- The `DOING` column is now `DETAIL`, in the TUI and the web dashboard alike. The
  name no longer described its contents: three of the five things it carries are
  not actions (a permission prompt's subject, `shell`, a call message) and a
  fourth is the negation of one (`interrupted`). It also removes a real ambiguity
  — a *different* column was already called `ACT`/`Activity` (the token
  sparkline). The API field follows: `GET /api/sessions` now returns `detail`
  instead of `activity`. That is a contract change, and it is coordinated rather
  than silent: the TUI and the daemon are already version-locked to each other
  (the startup preflight), the web dashboard is served by the daemon itself, and
  the report endpoint still accepts the old `activity` field from a hook reporter
  that predates the rename — the one client deliberately exempt from the version
  gate. A saved column layout is migrated, so a renamed column keeps its position
  and stays hidden if you had hidden it (#393).
- A watcher whose build does not match the daemon can no longer write session
  state. Enforcement lives in the daemon — the watcher's build already travels in
  every report (#356), and a rule applied only by the client can be skipped by
  exactly the outdated client it must stop — which closes the gap where a machine
  running a watcher but never a TUI drifted unchecked. A refused report answers
  `409` and writes nothing, while the machine and its faulty build stay visible in
  `GET /api/watcher` and the Machines tab, so the operator can see what to
  upgrade. The watcher goes **inert** rather than exiting (the packaged unit uses
  `Restart=on-failure`, so exiting would crash-loop): it logs the drift once,
  retries a single report every 60 s, and resumes on its own once the builds
  realign. Hook reports stay ungated on purpose — they run inside the operator's
  Claude session ([design](docs/design/version-consistency.md), #384).

### Fixed

- A machine whose watcher is running but currently has **no session to report** no
  longer reads as having no watcher. Liveness was a side effect of session data —
  the server only refreshed a machine's heartbeat while handling a session report —
  so a machine with nothing open (or nothing newer than `--max-age`) silently
  dropped out of `GET /api/watcher`, showed as "hooks only" in the Machines tab,
  and made the TUI preflight refuse to start while blaming the server. The watcher
  now claims liveness on its own, every 5 s, over a dedicated
  `POST /api/watcher/heartbeat` that is independent of sessions
  ([design](docs/design/watcher-liveness.md), #386). That heartbeat also carries
  the version verdict, replacing the 60 s report-retry probe from #384 — which
  could never work on the machine this fixes, since a drifted watcher with no
  sessions had no report to probe with.

## [0.4.1] - 2026-08-08

### Fixed

- The daemon no longer returns intermittent `500`s on session and usage reports
  under load. `busy_timeout` and `foreign_keys` are per-connection SQLite state
  but were set on only the first pooled connection, so every other connection the
  pool opened had `busy_timeout=0` and failed a contended write immediately with
  `SQLITE_BUSY` — and the watcher writes every session (plus usage) every ~2 s.
  The pragmas now travel in the DSN, so the driver applies them to every
  connection and contending writers wait instead of erroring (#372).
- The TUI startup preflight no longer reports a running watcher as down. A stale
  server heartbeat is a failed round-trip, not proof the watcher is dead — a
  just-restarted watcher or an unreachable server looks identical. The preflight
  now cross-checks a local `/proc` liveness signal: it says "watcher not running,
  start it" only when no local watcher process exists, and otherwise points at the
  server/connectivity and says to retry (#371).

## [0.4.0] - 2026-08-07

### Changed

- `vigie watch` now owns the reporting-hooks lifecycle: it re-installs its own
  leg into `~/.claude/settings.json` at startup, so the installed hooks always
  match the running watcher — a service restart after an upgrade self-heals stale
  hooks (e.g. a missing `PreCompact` or a moved binary). The settings write is now
  atomic (temp + rename); a refresh failure is logged and never stops the watch
  ([ADR-0009](docs/adr/0009-watcher-managed-hooks.md)). Manual `vigie init` /
  `vigie hooks` still work.

### Fixed

- `vigie hooks` no longer advertises the removed `--detailed` flag in its usage,
  and an unknown flag now prints the real usage instead of a bare "Usage of hooks:".

### Added

- `vigie tui` now runs a startup preflight before entering the alt-screen: it
  requires a reachable server, a valid token, and a daemon whose build strictly
  matches the client's (commit-compared when either side is a `dev` build). Any
  failure prints both versions and the remediation and exits 1 — no more silent
  degradation behind a full-screen UI, and no bypass flag
  ([design](docs/design/tui-preflight.md)). When the local machine has vigie hooks
  installed, the preflight also requires a fresh, version-matching local watcher —
  a hooks-only machine with a dead or outdated watcher reports stale statuses.
- The Machines tab now shows each machine's watcher version. The watcher reports
  its build in the heartbeat, the server stores it per machine and returns it from
  `GET /api/watcher`, and both the TUI (a VERSION column) and the web dashboard
  (a per-machine chip) display it — so a watcher that has drifted behind the
  daemon is visible.
- Sessions the operator interrupted (Ctrl-C/Esc) now show an `interrupted` marker
  in the activity column instead of a bare `idle`, so a turn killed mid-flight is
  distinguishable from one that finished cleanly. It clears with no timer — the
  next real prompt or reply replaces it. A DOING refinement, not a new status.

## [0.3.0] - 2026-08-07

### Added

- New `compacting` status: while a session summarizes its context (a silent
  ~90–170 s the registry reports as `busy`), vigie now shows `compacting` instead
  of an opaque `working`, so the context-gauge drop becomes legible. Detected via
  a new `PreCompact` hook (start) and the transcript's `compact_boundary` (end),
  with a safety timeout; it is a display refinement of `working`, not an
  attention status ([ADR-0008](docs/adr/0008-compacting-status.md)).
- The `vigie` client and `vigied` daemon build versions are now visible in the
  dashboards: a Build section in the TUI Settings tab shows both and flags a
  client/daemon drift, and a `vigied <version>` chip sits in the web topbar
  (commit and build time in its tooltip). Served over a new `GET /api/version`.

### Fixed

- A session whose only work runs inside async subagents (the `Task`/`Agent` tool)
  now reads `working`, not `idle`. Vigie tracks in-flight subagents from the
  parent transcript alone — opening on the launch, closing on its
  `task-notification` — with a liveness cap that self-heals a missed close, and
  shows `N agents: <description>` in the activity column.

## [0.2.0] - 2026-08-05

### Changed

- TUI: tightened the fixed-width columns (`TOTAL`, `EFFORT`, `OUT`, `CTX`, `RC`)
  to their real content, reclaiming ~8 columns of width so more fit before the
  table overflows on a narrow terminal. No content is truncated.

### Fixed

- TUI summary strip now drops its trailing elements whole — activity first, then
  rc, then out — when the terminal is too narrow, instead of clamping and cutting
  the activity sparkline mid-glyph. The status counts are always kept.
- TUI bottom usage/platform strip no longer overflows a narrow terminal: it now
  drops the secondary platform indicator when the two don't fit and clamps the
  usage side as a last resort. The width-sweep scaling guard's fixture is now
  fully populated (usage, platform, activity history), so it actually exercises —
  and would catch — this class of overflow.
- TUI Machines and Settings tabs no longer overflow the terminal width on a
  narrow terminal: the machines overview table clamps each row to width, its
  no-watcher help text wraps, and the column-picker header wraps. The width-sweep
  scaling guard now covers all three interactive tabs.
- TUI Sessions view no longer overflows the terminal width on resize: the
  summary strip keeps its status counts and drops the secondary sort/connection
  side when space is tight, the key-hint footer wraps, and the column auto-drop
  now accounts for the row's left gutter so the table never spills over by a
  column or two.
- TUI: the "N columns hidden" overflow banner now wraps to the terminal width
  instead of running past the edge and being cut off on a narrow terminal — the
  message that explains the narrowness is no longer itself unreadable.
- Column picker: hiding a column now keeps its position instead of jumping it to
  the bottom, and reordering works on any column — hidden or visible (TUI + web).
- TUI column picker is now width-aware: it shows the width budget, flags every
  selected column the terminal is too narrow to fit, and a banner names the
  columns dropped instead of hiding them silently (the TUI never scrolls
  sideways).
- TUI `a` key now toggles the persistent "hide ended" setting directly (saved,
  shared with Settings) instead of a separate transient override that could
  diverge from it and appear broken.

## [0.1.0] - 2026-08-04

First release. claude-vigie is an observe-only monitor for Claude Code sessions
across machines — it reads and reports session state; it never drives a session
([ADR-0005](docs/adr/0005-observe-only.md)) and holds no operator-handling state
([ADR-0007](docs/adr/0007-read-only-to-operator.md)).

### Added

- **Architecture** — two static, CGO-free Go binaries: the daemon `vigied` (HTTP
  + SSE API, embedded SQLite) and the client `vigie` (hooks reporter, transcript
  watcher, terminal dashboard). One server, a client on every machine.
- **Session model** — seven live statuses (working, thinking, waiting, stalled,
  idle, error, ended) plus `stale` for a session on a machine with no watcher.
  Status and presence come from Claude Code's own session registry and
  transcripts; two observers (hooks + watcher) reconciled by authority.
- **Terminal dashboard (TUI)** — sessions table with per-session status, tokens,
  activity, and a detail view; single-glyph status indicators; sort, filter,
  group, and column select/reorder — all persisted.
- **Web dashboard** — a read-only browser mirror of the TUI, served by the
  daemon, with the same status/sort/column controls (client-local).
- **Attention** — desktop notifications when a session starts waiting, a
  next-waiting hotkey, machines-tab flagging of hosts running on hooks alone, and
  a GNOME Shell top-bar indicator.
- **Per-session insight** — reasoning effort, context-window usage percentage,
  permission mode (manual/accept/plan/auto/bypass), and the remote-control resume
  URL, all detected read-only.
- **Operations** — Prometheus metrics on a dedicated, unauthenticated ops
  listener (never the API port), a portable Grafana dashboard, and Claude platform
  status polled from status.claude.com.
- **Landing site** — the static claudevigie.org one-pager.

### Security

- The API binds `127.0.0.1` by default; every `/api/*` route is behind a
  constant-time shared-token check; request bodies are size-capped.

[Unreleased]: https://github.com/haribo/claude-vigie/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/haribo/claude-vigie/compare/v0.4.1...v0.5.0
[0.4.1]: https://github.com/haribo/claude-vigie/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/haribo/claude-vigie/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/haribo/claude-vigie/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/haribo/claude-vigie/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/haribo/claude-vigie/releases/tag/v0.1.0
