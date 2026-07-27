# Architecture

Claude Fleet is a central server plus two clients (web and terminal) that show
which Claude Code sessions are running across machines, what they are doing,
and how many tokens they consume.

## Overview

```
Claude Code hook  →  claude-fleet report  →  POST /api/report  →  SQLite
   (settings.json)      (reads the JSONL          (server)           │
                         transcript for tokens)                      ▼
Web dashboard / TUI  ←──────── SSE /api/events ←────────────── current state
```

## Binaries

Two binaries, split along the deployment boundary (see
[ADR-0003](adr/0003-split-client-and-daemon-binaries.md)):

- **`claude-fleetd`** — the server daemon. Runs on the host machine. Carries
  SQLite, the HTTP server, and the embedded web dashboard.
- **`claude-fleet`** — the client. Installed on every machine running Claude
  Code sessions. Stays minimal (no server code, no SQLite).

## Components

| Component | Binary | Subcommand | Role |
|-----------|--------|------------|------|
| Server | `claude-fleetd` | `serve` | HTTP API + embedded web dashboard, SQLite storage, SSE stream |
| Installer | `claude-fleet` | `init` | Merges hooks into `~/.claude/settings.json`, writes the client config |
| Reporter | `claude-fleet` | `report` | Invoked by Claude Code hooks; sends events to the server |
| Terminal client | `claude-fleet` | `tui` | Live dashboard in the terminal (Bubble Tea) |

## Unit of tracking

The unit is **one Claude Code session**, identified by the `session_id` the
hooks provide. Sessions are grouped in the UI by machine and project directory.

## Configuration & install

Two files, kept separate:

- **Hooks** → `~/.claude/settings.json` (user level). One install covers every
  Claude Code session on the machine, present and future.
- **Client config** → `~/.config/claude-fleet/config.json` (XDG). Holds the
  server URL, the shared auth token, and the machine name. Never committed —
  it contains a secret. Written by `claude-fleet init`, read by `report`/`tui`.

## Hooks → status mapping

| Claude Code hook | Effect |
|------------------|--------|
| `SessionStart` | Session appears (machine, project, git branch, model) |
| `UserPromptSubmit` | status → `working` |
| `PostToolUse` | last tool used + heartbeat |
| `Notification` | status → `waiting` |
| `Stop` | status → `idle` + push token usage (read from the transcript) |
| `SessionEnd` | status → `ended` (archived to history) |

## Usage data

Token counters (input / output / cache, per model) are read from the session
transcript JSONL (`transcript_path`, provided by the hooks). Cost in currency
is intentionally out of scope — tokens only.

## Server API (planned)

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| POST | `/api/report` | token | Receive a session event |
| GET | `/api/sessions` | token | Current state of all sessions |
| GET | `/api/events` | token | SSE stream of state changes |
| GET | `/` | — | Web dashboard (static, embedded) |

## Auth

A single shared token (v1). The reporter and clients send it in the
`Authorization` header; the server rejects requests without it. Suitable for
personal use or a small trusted team. Per-machine tokens may come later.

## Storage

SQLite, a single file managed by the server. Holds current session state and
historical usage. Zero external database to deploy — the whole system is one
binary plus a `.db` file.
