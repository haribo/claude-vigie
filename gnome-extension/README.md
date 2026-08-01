# Claude Vigie — GNOME Shell extension

A top-bar indicator for [vigie](../README.md): it shows, at a glance, how
many Claude Code sessions across your fleet are **waiting for input**, and lists
sessions grouped by status in a dropdown.

It is a **read-only** client of a `vigied` server — it polls
`GET /api/sessions` and never writes into or drives a session (observe-only, see
[ADR-0005](../docs/adr/0005-observe-only.md)).

> Scope: this first version is the **indicator**. A push notification when a
> session enters `waiting` is the follow-up (issue #64).

## Requirements

- GNOME Shell 45–50 (ESM extensions).
- A reachable `vigied` server and its token.

## Install (from source)

```bash
cd gnome-extension
gnome-extensions pack --force \
  --extra-source=icons \
  --schema=schemas/org.gnome.shell.extensions.claude-vigie.gschema.xml
gnome-extensions install --force claude-vigie@haribo.github.io.shell-extension.zip
```

Then log out/in (Wayland) or restart the Shell (`Alt+F2`, `r`, X11), and enable it:

```bash
gnome-extensions enable claude-vigie@haribo.github.io
```

## Configure

Open the extension preferences (via *Extensions* app, or
`gnome-extensions prefs claude-vigie@haribo.github.io`) and set:

- **Server URL** — e.g. `https://fleet.example.com` (or `http://localhost:8080`).
- **Token** — the shared fleet token.
- **Poll interval** — seconds between refreshes (default 5).

## Behavior

- The radar icon shows a **count badge** and turns to the attention color when at
  least one session is `waiting`.
- Clicking it opens a dropdown listing sessions grouped by status
  (`working` / `waiting` / `idle` / `ended`) with project, machine, and branch.
- If the server is unreachable or the token is wrong, the icon dims and the menu
  shows the reason.

## Development

The extension is plain GJS (no build step). After editing, re-pack and re-install
with the commands above, then restart the Shell. Logs:

```bash
journalctl --user -f -o cat /usr/bin/gnome-shell   # runtime (extension.js)
journalctl --user -f -o cat | grep -i vigie  # our console.debug lines
```
