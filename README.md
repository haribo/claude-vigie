# Claude Fleet

Monitor your Claude Code sessions across machines from a terminal dashboard. See
which sessions are running, what they're working on, and how many tokens they
consume.

> **Status: v0.1 — functional.** The server, reporter, watcher, and terminal
> dashboard (TUI) work end to end. A web dashboard is planned; today the TUI is
> the client. See the [roadmap](#roadmap).

## How it works

Claude Fleet is a central server and a client you install on every machine. Each
Claude Code session reaches the server two ways: through **hooks** — a small
`claude-fleet report` command wired into `~/.claude/settings.json` — and through
a **watcher** that scans transcripts to cover sessions the hooks miss.

```
Claude Code hook  →  claude-fleet report  ┐
                                          ├─→  POST /api/report  →  claude-fleetd  →  SQLite
claude-fleet watch (scans transcripts)  ──┘        (server)              │
                                                                         ▼
              Terminal dashboard (TUI)  ←──── SSE /api/events ←──── current state
```

The unit of tracking is one Claude Code session. Sessions are grouped by
machine and project. See [docs/architecture.md](docs/architecture.md) for the
full design.

## Two binaries

```
claude-fleetd serve     # server: HTTP + SSE API, SQLite — runs on the host
claude-fleetd token     # server: print/generate the shared auth token
claude-fleet  init      # client: install hooks + write the config
claude-fleet  report    # client: reporter invoked by Claude Code hooks
claude-fleet  watch     # client: watcher — scans transcripts, covers all sessions
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
just app-serve       # runs claude-fleetd (foreground; add fleet_port=9090 for another port)

# on each machine running Claude Code (another terminal):
just fleet-connect   # writes config + hooks; reads the token from the local db
```

`fleet-connect` runs `claude-fleet init`, which merges hooks into
`~/.claude/settings.json` (all projects) and writes
`~/.config/claude-fleet/config.json`. New Claude Code sessions then report
automatically. `just fleet-disconnect` removes the hooks.

Hooks only cover sessions started *after* they're installed. To see **all** your
sessions (including ones already open), run the watcher, which scans
`~/.claude/projects/` and reports every recent session:

```bash
just app-watch       # keep running; covers all local sessions
```

The hooks refine real-time status; the watcher guarantees coverage.

### Run as background services (systemd, Linux)

The **server** is central (one host); the **watcher** runs on **every machine**
with Claude sessions. Install only what each machine needs:

```bash
# on the host that runs the server:
just fleet_port=9090 fleet-server-install

# on every machine running Claude sessions (including the host, if it runs Claude):
just fleet-watch-install
just fleet_port=9090 fleet-connect     # config + hooks: point the client at the server

journalctl --user -u claude-fleet-watch -f            # follow the watcher logs
just fleet-server-uninstall                           # remove the server service
just fleet-watch-uninstall                            # remove the watcher service
```

The watcher reads the client config, so run `fleet-connect` once so it knows the
server URL and token.

Port 8080 already taken? Override it (same value for both recipes):
`just fleet_port=9090 app-serve` and `just fleet_port=9090 fleet-connect`.

For a remote server, point init at it directly:
`claude-fleet init --server <url> --token <token>`.

## Roadmap

- [x] Client (`claude-fleet`) / daemon (`claude-fleetd`) split, SQLite store
- [x] Server: `/api/report`, `/api/sessions`, Bearer auth, SSE realtime stream
- [x] Reporter: hook payload + transcript parsing
- [x] `init`: merge hooks into `settings.json`, write config
- [x] Watcher: transcript scan + process-presence status (idle vs ended)
- [x] Terminal client (Bubble Tea): sort, filter, group, detail, settings
- [x] Subscription usage (5-hour / 7-day), single leased fetcher
- [x] Remote-control (`/rc`) detection, observe-only
- [x] Session retention / pruning
- [ ] Web dashboard (embedded)

## Contributing

Issue-first workflow, Conventional Commits, PRs target `develop`. See
[docs/git-workflow.md](docs/git-workflow.md), [docs/git-commits.md](docs/git-commits.md),
and [docs/git-issues.md](docs/git-issues.md).

## License

[MIT](LICENSE)
