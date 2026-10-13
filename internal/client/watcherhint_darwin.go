package client

// watcherRestart is how `vigie init` tells the operator to (re)start the watcher:
// the LaunchAgent from docs/deployment.md, whose label it must name (ADR-0018).
const watcherRestart = "launchctl kickstart -k gui/$(id -u)/local.vigie.watch"
