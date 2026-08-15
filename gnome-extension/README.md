# Claude Vigie — GNOME Shell extension

A top-bar indicator for [vigie](../README.md): it shows, at a glance, how
many Claude Code sessions across your fleet are **calling for you**, lists
sessions grouped by status in a dropdown, and **pushes a notification** when a
session starts calling — so a blocked session reaches you whether or not you're
looking at the bar.

It is a **read-only** client of a `vigied` server — it polls
`GET /api/sessions` and never writes into or drives a session (observe-only, see
[ADR-0005](../docs/adr/0005-observe-only.md)).

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
- **Desktop notifications** — notify when a session starts calling for you
  (default on).

## Behavior

- The radar icon shows a **count badge** and turns to the attention color when at
  least one session is calling for you.
- A **notification** fires when a session **starts calling for you** — it began
  `waiting` on input, a tool hung (`stalled`), it hit an API error, or the session
  raised a call of its own ([ADR-0010](../docs/adr/0010-session-raised-operator-call.md)).
  The body says which. Edge-triggered — once per transition, not every poll. The
  first poll after launch only seeds state, so enabling the extension never
  notifies for sessions that were already calling. Toggle it off in preferences.
- Clicking the icon opens a dropdown listing sessions grouped by status, with
  project, machine, and branch. Every status the server can return is shown, in
  the order [`session-status.md`](../docs/design/session-status.md) § 1 lists them:
  `working`, `thinking`, `compacting`, `waiting`, `stalled`, `idle`, `error`,
  `stale`, `ended`. A status this extension does not recognise is appended rather
  than dropped, so one added on the server side arrives unstyled instead of taking
  its sessions off the screen (#422).
- If the server is unreachable or the token is wrong, the icon dims and the menu
  shows the reason.

## Development

The extension is plain GJS (no build step). After editing, re-pack and re-install
with the commands above, then restart the Shell. Logs:

```bash
journalctl --user -f -o cat /usr/bin/gnome-shell   # runtime (extension.js)
journalctl --user -f -o cat | grep -i vigie  # our console.debug lines
```
