<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/wordmark-dark.svg">
    <img src="docs/assets/wordmark.svg" alt="Claude Vigie" height="72">
  </picture>
</p>

<p align="center">
  Monitor your Claude Code sessions across machines from a terminal dashboard —<br>
  which sessions are running, what they're working on, and how many tokens they consume.
</p>

<p align="center">
  <a href="https://github.com/haribo/claude-vigie/releases/latest"><img src="https://img.shields.io/github/v/release/haribo/claude-vigie?sort=semver&color=D9663F" alt="Latest release"></a>
  <a href="https://github.com/haribo/claude-vigie/actions/workflows/ci.yaml"><img src="https://img.shields.io/github/actions/workflow/status/haribo/claude-vigie/ci.yaml?branch=develop&label=CI" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/haribo/claude-vigie?color=blue" alt="License: MIT"></a>
</p>

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/hero-dark.svg">
    <img src="docs/assets/hero.svg" alt="vigie TUI — the Sessions tab: per-session status, tokens, and subscription usage" width="900">
  </picture>
</p>

## How it works

Claude Vigie is a central server and a client you install on every machine. Each
Claude Code session reaches the server two ways: through **hooks** — a small
`vigie report` command wired into `~/.claude/settings.json` — and through
a **watcher** that scans transcripts to cover sessions the hooks miss.

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/how-it-works-dark.svg">
    <img src="docs/assets/how-it-works.svg" alt="Claude Vigie architecture: hooks and a watcher on every machine POST to vigied, which stores to SQLite and streams to the TUI over SSE" width="820">
  </picture>
</p>

The unit of tracking is one Claude Code session. Sessions are grouped by
machine and project. See [docs/architecture.md](docs/architecture.md) for the
full design.

## Two binaries

```
vigied serve     # server: HTTP + SSE API, SQLite — runs on the host
vigied token     # server: print/generate the shared auth token
vigie  init      # client: install hooks + write the config
vigie  hooks     # client: add/remove reporting hooks (one leg per FLEET_CONFIG)
vigie  report    # client: reporter invoked by Claude Code hooks
vigie  watch     # client: watcher — scans transcripts, covers all sessions
vigie  tui       # client: terminal dashboard
```

`vigied` is the server daemon (one host). `vigie` is the client
you install on every machine running Claude Code sessions. See
[ADR-0003](docs/adr/0003-split-client-and-daemon-binaries.md).

The daemon also serves a **read-only web dashboard** at its root URL — a browser
mirror of the TUI. Open it, paste the server token, and watch every machine
from a phone or laptop. No extra process: it is embedded in `vigied`.

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

vigie ships **binaries** (`vigie`, `vigied`). How you run
and expose them — systemd, containers, TLS front — is the deployer's call; see
[docs/deployment.md](docs/deployment.md).

```bash
# on the host: run the server
vigied serve --addr :8080 --db vigie.db

# on each machine running Claude Code: connect (writes config + installs hooks)
vigie init --server http://host:8080 --token <token> --machine "$(hostname)"

# cover already-open sessions too (run this as a service):
vigie watch

# view the board in the terminal:
vigie tui

# ...or in a browser: open http://host:8080 and paste the token
```

Hooks cover sessions started *after* `init`; the watcher guarantees coverage by
scanning `~/.claude/projects/`. `vigie init --uninstall` removes the hooks.
The client reads `~/.config/vigie/config.toml` (override the path with
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
just dev-server     # background: builds & runs vigied on :8099 (dev db + token)
just dev-watcher    # background: watcher → the dev server
just dev-tui        # foreground: the TUI → the dev server
just dev-down       # stop the background dev server + watcher
```

## Disclaimer

vigie is an independent, community project. It is **not** affiliated
with, endorsed by, or sponsored by Anthropic. "Claude" and "Claude Code" are
trademarks of Anthropic, PBC, used here only to describe interoperability.

## License

[MIT](LICENSE)
