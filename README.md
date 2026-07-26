# Claude Fleet

Monitor your Claude Code sessions across machines — from a web dashboard or the
terminal. See which sessions are running, what they're working on, and how many
tokens they consume.

> **Status: early development.** The project skeleton, tooling, and conventions
> are in place. The server, reporter, and clients are being implemented feature
> by feature. See the [roadmap](#roadmap).

## How it works

Claude Fleet is a central server plus two clients. Each Claude Code session
reports to the server through **hooks** — a small `claude-fleet report` command
wired into `~/.claude/settings.json`, so it works automatically for every
session on a machine.

```
Claude Code hook  →  claude-fleet report  →  POST /api/report  →  SQLite
   (settings.json)      (reads the JSONL          (server)           │
                         transcript for tokens)                      ▼
Web dashboard / TUI  ←──────── SSE /api/events ←────────────── current state
```

The unit of tracking is one Claude Code session. Sessions are grouped by
machine and project. See [docs/architecture.md](docs/architecture.md) for the
full design.

## Two binaries

```
claude-fleetd serve     # server: HTTP API + web dashboard (SQLite, SSE) — runs on the host
claude-fleet  init      # client: install hooks + write the config
claude-fleet  report    # client: reporter invoked by Claude Code hooks
claude-fleet  tui       # client: terminal dashboard
```

`claude-fleetd` is the server daemon (one host). `claude-fleet` is the client
you install on every machine running Claude Code sessions. See
[ADR-0003](docs/adr/0003-split-client-and-daemon-binaries.md).

## Design choices

- **Go, static binaries** — trivial self-hosting, cross-platform, minimal
  client surface (client and server are separate binaries). See
  [ADR-0002](docs/adr/0002-single-go-binary-with-sqlite.md) and
  [ADR-0003](docs/adr/0003-split-client-and-daemon-binaries.md).
- **Embedded SQLite** — no database server to deploy; full usage history.
- **Shared-token auth** — simple, suitable for personal use or a small team.
- **Tokens only** — no currency cost estimates (Claude Code subscriptions have
  no per-token price).

## Development

Requires Go 1.26+ and [`just`](https://github.com/casey/just).

```bash
just dev-setup      # configure git hooks
just tool-install   # install golangci-lint + goimports into ./bin
just app-build      # build ./bin/claude-fleet
just code-check     # fmt + lint + build + test (run before every PR)
```

## Self-hosting & connecting

Install the binaries and connect a machine with `just`:

```bash
just install         # build + copy claude-fleet & claude-fleetd into ~/.local/bin

# on the host that runs the server:
just fleet-serve     # runs claude-fleetd on 127.0.0.1:8080 (foreground)

# on each machine running Claude Code (another terminal):
just fleet-connect   # writes config + hooks; reads the token from the local db
```

`fleet-connect` runs `claude-fleet init`, which merges hooks into
`~/.claude/settings.json` (all projects) and writes
`~/.config/claude-fleet/config.json`. New Claude Code sessions then report
automatically. `just fleet-disconnect` removes the hooks.

For a remote server, point init at it directly:
`claude-fleet init --server <url> --token <token>`.

## Roadmap

- [x] Project skeleton, tooling, CI, conventions
- [x] Split into client (`claude-fleet`) and daemon (`claude-fleetd`) binaries
- [x] Client config (XDG) load/save
- [x] SQLite store (sessions, events)
- [x] Server: `/api/report`, `/api/sessions`, Bearer auth
- [ ] Server: SSE realtime stream
- [x] Reporter: hook payload + transcript parsing
- [x] `init`: merge hooks into `settings.json`, write config
- [ ] Web dashboard (embedded)
- [x] Terminal client (Bubble Tea)

## Contributing

Issue-first workflow, Conventional Commits, PRs target `develop`. See
[docs/git-workflow.md](docs/git-workflow.md), [docs/git-commits.md](docs/git-commits.md),
and [docs/git-issues.md](docs/git-issues.md).

## License

[MIT](LICENSE)
