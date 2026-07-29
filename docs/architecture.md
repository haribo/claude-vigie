# Architecture

Claude Fleet shows which Claude Code sessions are running across machines, what
they are doing, and how many tokens they consume. This is the top-level map;
observable behavior is specced under [`design/`](design/) and decisions under
[`adr/`](adr/).

## Overview

```
Claude Code hook  →  claude-fleet report  ┐
                                          ├─→  POST /api/report  →  claude-fleetd  →  SQLite
claude-fleet watch (scans transcripts)  ──┘        (server)              │
                                                                         ▼
              Terminal dashboard (TUI)  ←──── SSE /api/events ←──── current state
```

Two independent paths feed the server: **hooks** (event-driven, precise) via
`report`, and the **watcher** (polling, coverage) via `watch`. How their status
signals are reconciled is specced in
[`design/session-status.md`](design/session-status.md).

## Binaries

Two binaries, split along the deployment boundary (see
[ADR-0003](adr/0003-split-client-and-daemon-binaries.md),
[ADR-0004](adr/0004-when-to-split-a-binary.md)):

- **`claude-fleetd`** — the server daemon. One per fleet, on a host machine.
  Carries SQLite and the HTTP/SSE API. Subcommands: `serve`, `token`.
- **`claude-fleet`** — the client. Installed on every machine running Claude Code
  sessions. Stays minimal (no server code, no SQLite). Subcommands: `init`,
  `report`, `watch`, `tui`.

A fleet is *N* client machines all reporting to one daemon.

## Components

| Component | Binary | Subcommand | Role |
|-----------|--------|------------|------|
| Server | `claude-fleetd` | `serve` | HTTP + SSE API, SQLite storage, session pruning |
| Token | `claude-fleetd` | `token` | Print/generate the shared auth token (to connect clients) |
| Installer | `claude-fleet` | `init` | Merges hooks into `~/.claude/settings.json`, writes the client config |
| Reporter | `claude-fleet` | `report` | Invoked by Claude Code hooks; sends one event per hook |
| Watcher | `claude-fleet` | `watch` | Background service: scans local transcripts, derives status from process presence + activity, reports every session (covering ones the hooks miss), and holds the usage lease to fetch subscription usage |
| Terminal client | `claude-fleet` | `tui` | Live dashboard in the terminal (Bubble Tea) |

A **web dashboard** is planned but not yet built: the daemon serves the API only,
and the TUI is currently the sole client.

## Unit of tracking

The unit is **one Claude Code session**, identified by the `session_id` the hooks
provide. Sessions are grouped in the UI by machine and project directory (see
[`design/session-list.md`](design/session-list.md)).

## Configuration & install

Two files, kept separate:

- **Hooks** → `~/.claude/settings.json` (user level). One install covers every
  Claude Code session on the machine, present and future.
- **Client config** → `~/.config/claude-fleet/config.toml` (XDG). Holds the
  server URL, the shared auth token, and the machine name. Never committed — it
  contains a secret. Written by `claude-fleet init`, read by `report`/`watch`/`tui`.
  A pre-TOML `config.json` is migrated on first load.

The watcher and the daemon run as systemd user services
(`claude-fleet-watch.service`, `claude-fleetd.service`).

## Hooks

`claude-fleet init` installs these Claude Code hooks; each invokes
`claude-fleet report` with the event. The resulting status is derived and
reconciled per [`design/session-status.md`](design/session-status.md).

| Hook | Reports |
|------|---------|
| `SessionStart` | session appears (machine, project, git branch, model); records process presence ([ADR-0006](adr/0006-session-presence-via-proc.md)) |
| `UserPromptSubmit` | a turn started; backfills presence |
| `PostToolUse` | last tool used + heartbeat |
| `Notification` | Claude is waiting on the human |
| `Stop` | turn ended + token usage (read from the transcript) |
| `SessionEnd` | session closed; clears presence |

## Server API

All `/api/*` routes require the shared token in the `Authorization` header.

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/healthz` | Liveness probe (no auth) |
| POST | `/api/report` | Receive a session event (from hooks and the watcher) |
| GET | `/api/sessions` | Current state of all sessions |
| GET | `/api/events` | SSE stream of state changes |
| POST | `/api/usage/lease` | Acquire the fleet's usage-fetch lease |
| GET · POST | `/api/usage` | Read · post subscription usage (percentages only) |
| GET | `/api/watcher` | Last watcher heartbeat (drives the "no watcher" warning) |
| GET · POST | `/api/settings` | Read · update server settings (session retention) |

## Usage & remote control

- **Usage** — subscription budget (5-hour / 7-day windows, percentages only) is
  fetched by a single leased machine; the token never leaves it. Specced in
  [`design/usage.md`](design/usage.md).
- **Remote control** — the `/rc` state is *detected* read-only, never set;
  claude-fleet is observe-only. See
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
