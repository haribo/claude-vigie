<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/wordmark-dark.svg">
    <img src="docs/assets/wordmark.svg" alt="Claude Fleet" height="72">
  </picture>
</p>

<p align="center">
  Monitor your Claude Code sessions across machines from a terminal dashboard —<br>
  which sessions are running, what they're working on, and how many tokens they consume.
</p>

> **Status: v0.2 — functional.** The server, reporter, watcher, and terminal
> dashboard (TUI) — sessions, stats, and machines tabs — work end to end. A web
> dashboard is planned; today the TUI is the client. See the [roadmap](#roadmap).

## How it works

Claude Fleet is a central server and a client you install on every machine. Each
Claude Code session reaches the server two ways: through **hooks** — a small
`claude-fleet report` command wired into `~/.claude/settings.json` — and through
a **watcher** that scans transcripts to cover sessions the hooks miss.

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/how-it-works-dark.svg">
    <img src="docs/assets/how-it-works.svg" alt="Claude Fleet architecture: hooks and a watcher on every machine POST to claude-fleetd, which stores to SQLite and streams to the TUI over SSE" width="820">
  </picture>
</p>

The unit of tracking is one Claude Code session. Sessions are grouped by
machine and project. See [docs/architecture.md](docs/architecture.md) for the
full design.

## Two binaries

```
claude-fleetd serve     # server: HTTP + SSE API, SQLite — runs on the host
claude-fleetd token     # server: print/generate the shared auth token
claude-fleet  init      # client: install hooks + write the config
claude-fleet  hooks     # client: add/remove reporting hooks (one leg per FLEET_CONFIG)
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

## Install & run

claude-fleet ships **binaries** (`claude-fleet`, `claude-fleetd`). How you run
and expose them — systemd, containers, TLS front — is the deployer's call; see
[docs/deployment.md](docs/deployment.md).

```bash
# on the host: run the server
claude-fleetd serve --addr :8080 --db fleet.db

# on each machine running Claude Code: connect (writes config + installs hooks)
claude-fleet init --server http://host:8080 --token <token> --machine "$(hostname)"

# cover already-open sessions too (run this as a service):
claude-fleet watch

# view the fleet:
claude-fleet tui
```

Hooks cover sessions started *after* `init`; the watcher guarantees coverage by
scanning `~/.claude/projects/`. `claude-fleet init --uninstall` removes the hooks.
The client reads `~/.config/claude-fleet/config.toml` (override the path with
`FLEET_CONFIG`).

## Development

Requires Go 1.26+ and [`just`](https://github.com/casey/just).

```bash
just dev-setup      # configure git hooks
just tool-install   # install golangci-lint + goimports into ./bin
just code-check     # fmt + lint + build + test (run before every PR)
```

Run the current source against a throwaway local server, fully isolated from any
installed production client via `FLEET_CONFIG` (never touches `~/.config`):

```bash
just dev-server     # background: builds & runs claude-fleetd on :8099 (dev db + token)
just dev-watcher    # background: watcher → the dev server
just dev-tui        # foreground: the TUI → the dev server
just dev-down       # stop the background dev server + watcher
```

## Roadmap

- [x] Client (`claude-fleet`) / daemon (`claude-fleetd`) split, SQLite store
- [x] Server: `/api/report`, `/api/sessions`, Bearer auth, SSE realtime stream
- [x] Reporter: hook payload + transcript parsing
- [x] `init`: merge hooks into `settings.json`, write config
- [x] Watcher: transcript scan + process-presence status (idle vs ended)
- [x] Terminal client (Bubble Tea): sessions / stats / machines tabs — sort, filter, group, detail, settings
- [x] Subscription usage (5-hour / 7-day), single leased fetcher
- [x] Remote-control (`/rc`) detection, observe-only
- [x] Session retention / pruning
- [x] Analytics: daily rollups (`/api/stats`) — bottleneck time, tokens by model, top sessions
- [x] Per-machine overview (machines tab)
- [ ] Web dashboard (embedded)

## Contributing

Issue-first workflow, Conventional Commits, PRs target `develop`. See
[docs/git-workflow.md](docs/git-workflow.md), [docs/git-commits.md](docs/git-commits.md),
and [docs/git-issues.md](docs/git-issues.md).

## License

[MIT](LICENSE)
