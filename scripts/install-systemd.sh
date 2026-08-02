#!/usr/bin/env bash
# Install a systemd --user service for one fleet role.
# Usage:
#   install-systemd.sh server <bin_dir> <addr> <db>
#   install-systemd.sh watch  <bin_dir>
set -euo pipefail

ROLE="${1:?usage: install-systemd.sh <server|watch> <bin_dir> [addr] [db]}"
BIN_DIR="${2:?missing bin_dir}"
UNIT_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
mkdir -p "$UNIT_DIR"

case "$ROLE" in
  server)
    ADDR="${3:?missing addr}"
    DB="${4:?missing db}"
    cat > "$UNIT_DIR/vigied.service" <<EOF
[Unit]
Description=Claude Vigie server
After=network.target

[Service]
ExecStart=$BIN_DIR/vigied serve --addr $ADDR --db $DB
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
EOF
    systemctl --user daemon-reload
    systemctl --user enable vigied.service
    systemctl --user restart vigied.service
    echo "installed and started vigied.service ($ADDR)"
    ;;
  watch)
    cat > "$UNIT_DIR/vigie-watch.service" <<EOF
[Unit]
Description=Claude Vigie watcher
After=network.target

[Service]
ExecStart=$BIN_DIR/vigie watch
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
EOF
    systemctl --user daemon-reload
    systemctl --user enable vigie-watch.service
    systemctl --user restart vigie-watch.service
    echo "installed and started vigie-watch.service"
    ;;
  *)
    echo "unknown role: $ROLE (use 'server' or 'watch')" >&2
    exit 1
    ;;
esac
