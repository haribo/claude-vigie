# claudevigie.org — landing site

The public landing page for claude-vigie. A single static `index.html` (inline
CSS/JS, a `<canvas>` radar, no framework, no build step) plus `assets/`.

This is **only the marketing site**. It is unrelated to `vigied`, the daemon
that serves the in-product web dashboard (`internal/web/`) — do not confuse the
two.

## Preview locally

```bash
cd site && python3 -m http.server 8000   # then open http://localhost:8000
```

## Deploy

Served by Caddy from `/srv/claudevigie.org` (see `Caddyfile`). Pushing to
`develop` under `site/**` triggers `.github/workflows/deploy-site.yaml`, which
rsyncs `site/` to the host over SSH and is a no-op if the deploy secrets are
unset. Required repository secrets:

| Secret | Meaning |
|--------|---------|
| `DEPLOY_HOST` | host running Caddy |
| `DEPLOY_USER` | SSH user |
| `DEPLOY_SSH_KEY` | private key (the matching public key is authorized on the host) |
| `DEPLOY_PATH` | target dir, e.g. `/srv/claudevigie.org` |
| `DEPLOY_KNOWN_HOSTS` | `ssh-keyscan` output for the host |

To deploy by hand instead: `rsync -az --delete site/ user@host:/srv/claudevigie.org/`.
