# Changelog

All notable changes to claude-vigie are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project aims to
follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Each GitHub Release's notes are a mirror of that version's section below — this
file is the single source of truth, not a second narrative.

## [Unreleased]

### Fixed

- Column picker: hiding a column now keeps its position instead of jumping it to
  the bottom, and reordering works on any column — hidden or visible (TUI + web).

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
