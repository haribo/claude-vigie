# Changelog

All notable changes to claude-vigie are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project aims to
follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Each GitHub Release's notes are a mirror of that version's section below — this
file is the single source of truth, not a second narrative.

## [Unreleased]

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

[Unreleased]: https://github.com/haribo/claude-vigie/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/haribo/claude-vigie/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/haribo/claude-vigie/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/haribo/claude-vigie/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/haribo/claude-vigie/releases/tag/v0.1.0
