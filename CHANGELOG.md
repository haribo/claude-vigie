# Changelog

All notable changes to claude-vigie are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project aims to
follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Each GitHub Release's notes are a mirror of that version's section below — this
file is the single source of truth, not a second narrative.

## [Unreleased]

### Changed

- TUI: tightened the fixed-width columns (`TOTAL`, `EFFORT`, `OUT`, `CTX`, `RC`)
  to their real content, reclaiming ~8 columns of width so more fit before the
  table overflows on a narrow terminal. No content is truncated.

### Fixed

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

[Unreleased]: https://github.com/haribo/claude-vigie/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/haribo/claude-vigie/releases/tag/v0.1.0
