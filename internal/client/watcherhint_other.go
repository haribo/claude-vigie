//go:build !darwin

package client

// watcherRestart is how `vigie init` tells the operator to (re)start the watcher:
// the systemd user unit from docs/deployment.md.
const watcherRestart = "systemctl --user restart vigie-watch"
