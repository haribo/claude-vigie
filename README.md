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

## One binary, four subcommands

```
claude-fleet serve      # server + web dashboard (SQLite, SSE)
claude-fleet tui        # terminal dashboard
claude-fleet report     # reporter invoked by Claude Code hooks
claude-fleet init       # install hooks + write the client config
```

## Design choices

- **Go, single static binary** — trivial self-hosting, fast reporter startup
  (it runs on every hook), cross-platform. See
  [ADR-0002](docs/adr/0002-single-go-binary-with-sqlite.md).
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

## Roadmap

- [x] Project skeleton, tooling, CI, conventions
- [x] Client config (XDG) load/save
- [ ] SQLite store (sessions, events, usage)
- [ ] Server: `/api/report`, `/api/sessions`, SSE
- [ ] Reporter: hook payload + transcript parsing
- [ ] `init`: merge hooks into `settings.json`, write config
- [ ] Web dashboard (embedded)
- [ ] Terminal client (Bubble Tea)

## Contributing

Issue-first workflow, Conventional Commits, PRs target `develop`. See
[docs/git-workflow.md](docs/git-workflow.md), [docs/git-commits.md](docs/git-commits.md),
and [docs/git-issues.md](docs/git-issues.md).

## License

[MIT](LICENSE)
