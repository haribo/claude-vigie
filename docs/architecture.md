# Architecture

Claude Vigie shows which Claude Code sessions are running across machines, what
they are doing, and how many tokens they consume. This is the top-level map;
observable behavior is specced under [`design/`](design/) and decisions under
[`adr/`](adr/).

## Overview

```
Claude Code hook  →  vigie report  ┐
                                          ├─→  POST /api/report  →  vigied  →  SQLite
vigie watch (scans transcripts)  ──┘        (server)              │
                                                                         ▼
     Terminal dashboard (TUI) / web browser  ←──── SSE /api/events ←──── current state
```

Two independent paths feed the server: **hooks** (event-driven, precise) via
`report`, and the **watcher** (polling, coverage) via `watch`. How their status
signals are reconciled is specced in
[`design/session-status.md`](design/session-status.md).

## Binaries

Two binaries, split along the deployment boundary (see
[ADR-0003](adr/0003-split-client-and-daemon-binaries.md),
[ADR-0004](adr/0004-when-to-split-a-binary.md)):

- **`vigied`** — the server daemon. One per fleet, on a host machine.
  Carries SQLite, the HTTP/SSE API, and the embedded web dashboard. Subcommands:
  `serve`, `token`, `stats-repair`.
- **`vigie`** — the client. Installed on every machine running Claude Code
  sessions. Stays minimal (no server code, no SQLite). Subcommands: `init`,
  `hooks`, `report`, `call`, `watch`, `tui`.

A fleet is *N* client machines all reporting to one daemon.

## Components

| Component | Binary | Subcommand | Role |
|-----------|--------|------------|------|
| Server | `vigied` | `serve` | HTTP + SSE API, SQLite storage, session pruning, platform-status polling |
| Token | `vigied` | `token` | Print/generate the shared auth token (to connect clients) |
| Stats repair | `vigied` | `stats-repair` | Correct one day's output-token figure in the analytics table; daily stats are never recomputed, so a value corrupted by an earlier defect can only be set deliberately ([design](design/token-rollup.md)) |
| Configuration | `vigie` | `init` | Asks for the server URL, the token and the machine name, checks the connection, and writes the client config — nothing else. The watcher installs the hooks and the call skill ([ADR-0009](adr/0009-watcher-managed-hooks.md)) |
| Hooks | `vigie` | `hooks` | `install` / `uninstall` the reporting hooks and the call skill by hand, for a machine that runs no watcher |
| Reporter | `vigie` | `report` | Invoked by Claude Code hooks; sends one event per hook |
| Call | `vigie` | `call` | Run *inside* a session to raise a call for the operator ([ADR-0010](adr/0010-session-raised-operator-call.md)); cleared when that session resumes or ends. Claude learns the command from a personal Agent Skill vigie installs and refreshes ([design](design/call-discoverability.md)) |
| Watcher | `vigie` | `watch` | Background service: refreshes its own hooks at startup ([ADR-0009](adr/0009-watcher-managed-hooks.md)), scans local transcripts, derives status from process presence + activity, reports every session (covering ones the hooks miss), and holds the usage lease to fetch subscription usage |
| Terminal client | `vigie` | `tui` | Live dashboard in the terminal (Bubble Tea) |
| Web dashboard | `vigied` | `serve` | Read-only browser mirror of the TUI, served at `GET /` (static assets embedded via `go:embed`) |

The **web dashboard** is the second client, served by the daemon itself (no build
step, no framework — plain HTML/CSS/JS as `go:embed` static files, consistent with
the single-binary ethos of [ADR-0002](adr/0002-single-go-binary-with-sqlite.md)). Open the
daemon's URL in a browser and paste the fleet token; it is kept in the browser and
sent as a bearer token on same-origin API calls. Read-only, like every client
(observe-only, [ADR-0005](adr/0005-observe-only.md)).

## Unit of tracking

The unit is **one Claude Code session**, identified by the `session_id` the hooks
provide. Sessions are grouped in the UI by machine and project directory (see
[`design/session-list.md`](design/session-list.md)).

## Configuration & install

Two files, kept separate:

- **Hooks** → `~/.claude/settings.json` (user level). One install covers every
  Claude Code session on the machine, present and future.
- **Client config** → `~/.config/vigie/config.toml` (XDG). Holds the
  server URL, the shared auth token, and the machine name. Never committed — it
  contains a secret. Written by `vigie init`, read by `report`/`watch`/`tui`.
  A pre-TOML `config.json` is migrated on first load.

The watcher and the daemon run as systemd user services
(`vigie-watch.service`, `vigied.service`).

## Hooks

`vigie watch` installs these Claude Code hooks and **re-installs its own leg at
startup** ([ADR-0009](adr/0009-watcher-managed-hooks.md)); each invokes
`vigie report` with the event. A machine that runs no watcher installs them by
hand with `vigie hooks install`. So the installed hooks always match the running
watcher — a service restart after an upgrade self-heals stale hooks. The
resulting status is derived and reconciled per
[`design/session-status.md`](design/session-status.md).

| Hook | Reports |
|------|---------|
| `SessionStart` | session appears (machine, project, git branch, model); records process presence ([ADR-0006](adr/0006-session-presence-via-proc.md)) |
| `UserPromptSubmit` | a turn started; backfills presence |
| `PostToolUse` | last tool used + heartbeat + the "doing" activity message (installed by default) |
| `Notification` | `waiting` on a permission prompt, `idle` on an idle prompt (split by `notification_type`) |
| `Stop` | turn ended + token usage (read from the transcript) |
| `PreCompact` | context compaction started; drops a marker the watcher reads to refine `working`→`compacting` ([ADR-0008](adr/0008-compacting-status.md)) |
| `SessionEnd` | session closed; clears presence |

## Server API

All `/api/*` routes require the shared token in the `Authorization` header.

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/report` | Receive a session event (from hooks and the watcher) |
| GET | `/api/sessions` | Current state of all sessions |
| GET | `/api/events` | SSE stream of state changes |
| POST | `/api/usage/lease` | Acquire the fleet's usage-fetch lease |
| GET · POST | `/api/usage` | Read · post subscription usage (percentages only) |
| GET | `/api/watcher` | Last watcher heartbeat (drives the "no watcher" warning) |
| GET | `/api/status` | Claude platform health (server-polled from status.claude.com) |
| GET | `/api/stats` | Analytics rollups + top sessions |
| GET · POST | `/api/settings` | Read · update server settings (session retention) |

`/healthz` (liveness) and `/metrics` (Prometheus) are served on a **separate ops
listener** (`--metrics-addr`, default `127.0.0.1:9464`), not on the API port —
they carry no auth, so they stay off the token-protected surface. See
[deployment.md](deployment.md).

## Usage & remote control

- **Usage** — subscription budget (5-hour / 7-day windows, percentages only) is
  fetched by a single leased machine; the token never leaves it. Specced in
  [`design/usage.md`](design/usage.md).
- **Remote control** — the `/rc` state is *detected* read-only, never set;
  vigie is observe-only. See
  [`design/remote-control.md`](design/remote-control.md) and
  [ADR-0005](adr/0005-observe-only.md).

## Auth

A single shared token (v1). The reporter, watcher, and clients send it in the
`Authorization` header; the server rejects requests without it. Suitable for
personal use or a small trusted team. Per-machine tokens may come later. For
exposing the daemon safely (TLS front, localhost bind, token handling), see
[`deployment.md`](deployment.md).

## Storage

SQLite, a single file managed by the server (see
[ADR-0002](adr/0002-single-go-binary-with-sqlite.md)). Holds current session
state and historical usage samples. Zero external database to deploy — the whole
system is one binary plus a `.db` file.
