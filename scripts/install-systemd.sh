#!/usr/bin/env bash
# Generate and start systemd --user services for the fleet server and watcher.
# Usage: install-systemd.sh <bin_dir> <addr> <db>
set -euo pipefail

BIN_DIR="${1:?usage: install-systemd.sh <bin_dir> <addr> <db>}"
ADDR="${2:?missing addr}"
DB="${3:?missing db}"
UNIT_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"

mkdir -p "$UNIT_DIR"

cat > "$UNIT_DIR/claude-fleetd.service" <<EOF
[Unit]
Description=Claude Fleet server
After=network.target

[Service]
ExecStart=$BIN_DIR/claude-fleetd serve --addr $ADDR --db $DB
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
EOF

cat > "$UNIT_DIR/claude-fleet-watch.service" <<EOF
[Unit]
Description=Claude Fleet watcher
After=claude-fleetd.service
Wants=claude-fleetd.service

[Service]
ExecStart=$BIN_DIR/claude-fleet watch
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
EOF

systemctl --user daemon-reload
systemctl --user enable --now claude-fleetd.service claude-fleet-watch.service

echo "installed and started user services:"
echo "  claude-fleetd.service       ($ADDR)"
echo "  claude-fleet-watch.service"
echo "logs:   journalctl --user -u claude-fleet-watch -f"
echo "status: systemctl --user status claude-fleetd claude-fleet-watch"
