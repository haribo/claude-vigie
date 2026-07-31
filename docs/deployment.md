# Deployment

claude-fleet ships **binaries**. Running `claude-fleetd`, terminating TLS, and
exposing it are the **deployer's** responsibility — the project does not embed a
web server front, certificates, or an orchestrator. This guide states the
boundary and the security implications; it changes no defaults.

## The daemon

`claude-fleetd serve` is a single static binary + a SQLite file. Flags:

| Flag | Default | Purpose |
|------|---------|---------|
| `--addr` | `:8080` | listen address |
| `--db` | `claude-fleet.db` | SQLite file path |
| `--token` | — | shared auth token (else `$FLEET_TOKEN`, else the stored one, else generated) |
| `--session-retention` | `24h` | delete sessions not reported within this window (`0` disables) |
| `--metrics-addr` | `127.0.0.1:9090` | ops listener for `/metrics` and `/healthz` (empty disables) |

It is single-node by design (one SQLite writer, an in-memory SSE hub) — run **one
instance**, not replicas.

### Ops listener (metrics & health)

`/metrics` (Prometheus) and `/healthz` (liveness) are served on a **separate
listener** (`--metrics-addr`), never on the API port — so `:8080` stays purely
the token-protected API. The ops listener is **unauthenticated**; its safety comes
from the bind address (`127.0.0.1` by default). Expose it only to your scraper:

- **Same host / in-pod probe:** the localhost default is enough.
- **Remote Prometheus:** bind it to a reachable, scraper-only interface
  (`--metrics-addr 10.0.0.5:9090`) behind your network controls — not the public
  internet. It carries no session content: labels are bounded (`status`, `event`,
  `model`, `route`), never a session id, machine, or project.

Metrics are namespaced `fleet_*` (RED HTTP metrics, ingestion counters, a
scrape-time `fleet_sessions` gauge by reconciled status, SSE and prune counters,
DB size, watcher heartbeat) plus the default Go/process collectors.

A ready-made Grafana dashboard ships in [`dashboards/claude-fleet.json`](../dashboards/claude-fleet.json)
— import it and pick your Prometheus datasource (the dashboard uses a datasource
variable, so it is not tied to any instance).

## Local / trusted-LAN use

Plain HTTP is fine. Run it, point clients at `http://<host>:8080`, done. TLS only
matters once traffic crosses an untrusted network.

## Public exposure

If `fleetd` is reachable from the internet, two rules:

1. **Put a TLS front in front of it** (Caddy, nginx, Traefik). The front holds the
   certificate and forwards to `fleetd`. Clients talk `https://` to the front;
   `fleetd` stays plain HTTP on the host.
2. **Bind `fleetd` to `127.0.0.1`** so only the front (same host) reaches it:

   ```bash
   claude-fleetd serve --addr 127.0.0.1:8080
   ```

   Otherwise the default `:8080` also listens on the public interface, and someone
   can hit the raw HTTP port directly, **bypassing your TLS front**. They still
   need the token (every `/api/*` route requires it), but a legitimate client that
   ever reached that port in cleartext would leak the token. Close the door: bind
   localhost, let only the front be public.

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
- The client stores it in `~/.config/claude-fleet/config.toml` (written `0600`).
- It is a **single shared, static** credential with no per-machine revocation. If
  one machine leaks it, rotate everywhere.

## Clients (watcher / TUI / reporter)

Clients follow whatever scheme is in their `server_url`, so **set it to your
`https://` endpoint** and Go's HTTP client does TLS with certificate verification
against the OS trust store — no client-side TLS code, no flags.

- **Private / internal CA:** trust it at the OS level (or export `SSL_CERT_FILE` /
  `SSL_CERT_DIR`); Go respects it. There is deliberately **no** `--insecure`
  option — skipping verification would defeat the TLS entirely.
- `claude-fleet init --server https://fleet.example.com --token <token>` writes the
  client config; the watcher and TUI read it.

## Recommended for dynamic client IPs

If the watcher/TUI machines have changing IPs, a **private overlay network**
(Tailscale, WireGuard) is the simplest robust option: it gives each machine a
stable identity regardless of its public IP, and `fleetd` needs **no public port
at all** — zero internet attack surface, no certificate to manage. Reach for a
public TLS front only if you must serve clients that can't join the overlay.
