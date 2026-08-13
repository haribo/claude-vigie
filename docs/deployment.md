# Deployment

vigie ships **binaries**. Running `vigied`, terminating TLS, and
exposing it are the **deployer's** responsibility — the project does not embed a
web server front, certificates, or an orchestrator. This guide states the
boundary and the security implications; it changes no defaults.

## The daemon

`vigied serve` is a single static binary + a SQLite file. Flags:

| Flag | Default | Purpose |
|------|---------|---------|
| `--addr` | `127.0.0.1:8080` | listen address (bind a reachable interface for cross-machine clients) |
| `--db` | `vigie.db` | SQLite file path |
| `--token` | — | shared auth token (else `$FLEET_TOKEN`, else the stored one, else generated) |
| `--session-retention` | `24h` | delete sessions not reported within this window (`0` disables) |
| `--metrics-addr` | `127.0.0.1:9464` | ops listener for `/metrics` and `/healthz` (empty disables) |

It is single-node by design (one SQLite writer, an in-memory SSE hub) — run **one
instance**, not replicas.

### Ops listener (metrics & health)

`/metrics` (Prometheus) and `/healthz` (liveness) are served on a **separate
listener** (`--metrics-addr`), never on the API port — so `:8080` stays purely
the token-protected API. The ops listener is **unauthenticated**; its safety comes
from the bind address (`127.0.0.1` by default). Expose it only to your scraper:

- **Same host / in-pod probe:** the localhost default is enough.
- **Remote Prometheus:** bind it to a reachable, scraper-only interface
  (`--metrics-addr 10.0.0.5:9464`) behind your network controls — not the public
  internet. It carries no session content: labels are bounded (`status`, `event`,
  `model`, `route`), never a session id, machine, or project.

Metrics are namespaced `fleet_*` (RED HTTP metrics, ingestion counters, a
scrape-time `fleet_sessions` gauge by reconciled status, SSE and prune counters,
DB size, watcher heartbeat) plus the default Go/process collectors.

A ready-made Grafana dashboard ships in [`dashboards/vigie.json`](../dashboards/vigie.json)
— import it and pick your Prometheus datasource (the dashboard uses a datasource
variable, so it is not tied to any instance).

## Running under systemd (optional)

vigie does **not** install or manage systemd for you — driving the host's service
manager is the deployer's call, not the repo's. Here are copy-paste `--user` units
to run the daemon and the watcher yourself. Adjust the binary and db paths.

The server — `~/.config/systemd/user/vigied.service`:

```ini
[Unit]
Description=Claude Vigie server
After=network.target

[Service]
ExecStart=%h/.local/bin/vigied serve --addr 127.0.0.1:8080 --db %h/.local/share/vigie/vigie.db
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
```

Each machine's watcher — `~/.config/systemd/user/vigie-watch.service`:

```ini
[Unit]
Description=Claude Vigie watcher
After=network.target

[Service]
ExecStart=%h/.local/bin/vigie watch
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
```

Enable and start:

```bash
systemctl --user daemon-reload
systemctl --user enable --now vigied.service      # on the host
systemctl --user enable --now vigie-watch.service # on each machine
```

> **Make `--user` services survive logout.** A `--user` service is stopped when you
> log out unless lingering is enabled: `sudo loginctl enable-linger "$USER"`.
> Without it, a headless box's watcher (or daemon) dies at the end of your SSH
> session.

## Local / trusted-LAN use

The daemon binds `127.0.0.1` by default. For cross-machine clients, bind a
reachable interface explicitly (`--addr :8080`, or a specific LAN IP) and point
clients at `http://<host>:8080`. Plain HTTP is fine on a trusted network; TLS
only matters once traffic crosses an untrusted one.

## Public exposure

If `fleetd` is reachable from the internet, two rules:

1. **Put a TLS front in front of it** (Caddy, nginx, Traefik). The front holds the
   certificate and forwards to `fleetd`. Clients talk `https://` to the front;
   `fleetd` stays plain HTTP on the host.
2. **Keep `fleetd` on `127.0.0.1`** (the default) so only the front (same host)
   reaches it — no extra flag needed. Expose the raw port widely only by an
   explicit choice (`--addr :8080`); a public HTTP port would let someone hit it
   directly, **bypassing your TLS front**. They still need the token (every
   `/api/*` route requires it), but a legitimate client that ever reached that
   port in cleartext would leak the token. Leave the door closed: let only the
   front be public.

> **The shared token is only meaningful over TLS on a public network.** It travels
> in the `Authorization` header on every request; without TLS it is captured on
> the wire, and whoever captures it gets full read/write on the API. TLS is what
> makes the token worth anything here.

### Why this design is safe *with* the token

Every `/api/*` route is behind auth; a request with **no token or a wrong token**
gets `401` and no data (enforced by `TestEveryAPIRouteRejectsUnauthenticated`).
The token is 256-bit, compared in constant time — not guessable, no timing
oracle. The API port serves **only** authenticated `/api/*` routes; the
unauthenticated `/healthz` and `/metrics` live on the separate ops listener (see
above). So a public `fleetd` behind TLS leaks nothing to someone who does not
hold the token.

## The token

- Supply it as a secret via `FLEET_TOKEN` (or `--token`). If none is provided and
  none is stored, `fleetd` generates one and logs it once — fine for a first run,
  but prefer providing it explicitly in production.
- The client stores it in `~/.config/vigie/config.toml` (written `0600`).
- It is a **single shared, static** credential with no per-machine revocation. If
  one machine leaks it, rotate everywhere.

## Clients (watcher / TUI / reporter)

Clients follow whatever scheme is in their `server_url`, so **set it to your
`https://` endpoint** and Go's HTTP client does TLS with certificate verification
against the OS trust store — no client-side TLS code, no flags.

- **Private / internal CA:** trust it at the OS level (or export `SSL_CERT_FILE` /
  `SSL_CERT_DIR`); Go respects it. There is deliberately **no** `--insecure`
  option — skipping verification would defeat the TLS entirely.
- `vigie init` asks for the server URL, the token and this machine's name, checks
  the connection and writes the client config — and only that; the watcher
  installs the hooks and the call skill when it starts. The watcher and TUI read
  the config. It takes **no flags**: the token is read without echo, so it never
  reaches the shell history or `ps`. It needs a terminal, and says so rather than
  blocking when it has none.

## Recommended for dynamic client IPs

If the watcher/TUI machines have changing IPs, a **private overlay network**
(Tailscale, WireGuard) is the simplest robust option: it gives each machine a
stable identity regardless of its public IP, and `fleetd` needs **no public port
at all** — zero internet attack surface, no certificate to manage. Reach for a
public TLS front only if you must serve clients that can't join the overlay.
