# Default: list all commands
default:
    @just --list

# =============================================================================
# DEV SETUP
# =============================================================================

# Configure git hooks for local development
dev-setup:
    git config core.hooksPath .githooks
    @echo "git hooks configured (.githooks/)"

# Install dev tools (golangci-lint v2, goimports) into ./bin
tool-install:
    @mkdir -p bin
    GOBIN=$(pwd)/bin go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4
    GOBIN=$(pwd)/bin go install golang.org/x/tools/cmd/goimports@latest

# =============================================================================
# DEV RUN — run the current source against a throwaway local server, isolated
# from any installed production client via FLEET_CONFIG (never touches
# ~/.config). The background recipes track their pid in .dev/ and kill only that
# pid on restart — never by binary name, so a production watcher is never hit.
# =============================================================================

dev_dir := justfile_directory() / ".dev"
dev_bin := dev_dir / "bin"
dev_host := "127.0.0.1"
dev_port := "8099"
dev_addr := dev_host + ":" + dev_port
dev_url := "http://" + dev_host + ":" + dev_port
dev_config := dev_dir / "config.toml"
dev_db := dev_dir / "fleet.db"
dev_token := "dev"

# Write the dev client config (points the client at the local dev server).
dev-config:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p "{{dev_dir}}"
    printf 'server_url = "{{dev_url}}"\ntoken = "{{dev_token}}"\nmachine = "%s-dev"\n' "$(hostname)" > "{{dev_config}}"
    echo "wrote {{dev_config}}"

# Start the source server in the background (rebuild + restart, throwaway db).
dev-server:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p "{{dev_bin}}"
    go build -o "{{dev_bin}}/claude-fleetd" ./cmd/claude-fleetd
    pidf="{{dev_dir}}/server.pid"
    [ -f "$pidf" ] && kill "$(cat "$pidf")" 2>/dev/null || true
    nohup "{{dev_bin}}/claude-fleetd" serve --addr {{dev_addr}} --token {{dev_token}} --db "{{dev_db}}" --session-retention 0 > "{{dev_dir}}/server.log" 2>&1 &
    echo $! > "$pidf"
    echo "dev server → {{dev_url}} (pid $(cat "$pidf"), logs {{dev_dir}}/server.log)"

# Start the current-source watcher in the background, pointed at the dev server.
dev-watcher: dev-config
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p "{{dev_bin}}"
    go build -o "{{dev_bin}}/claude-fleet" ./cmd/claude-fleet
    pidf="{{dev_dir}}/watcher.pid"
    [ -f "$pidf" ] && kill "$(cat "$pidf")" 2>/dev/null || true
    FLEET_CONFIG="{{dev_config}}" nohup "{{dev_bin}}/claude-fleet" watch > "{{dev_dir}}/watcher.log" 2>&1 &
    echo $! > "$pidf"
    echo "dev watcher → {{dev_url}} (pid $(cat "$pidf"), logs {{dev_dir}}/watcher.log)"

# Run the current-source TUI in the foreground, pointed at the dev server.
dev-tui: dev-config
    FLEET_CONFIG="{{dev_config}}" go run ./cmd/claude-fleet tui

# Stop the background dev server and watcher.
dev-down:
    #!/usr/bin/env bash
    for name in server watcher; do
      pidf="{{dev_dir}}/$name.pid"
      if [ -f "$pidf" ]; then
        kill "$(cat "$pidf")" 2>/dev/null && echo "stopped dev $name" || true
        rm -f "$pidf"
      fi
    done

# =============================================================================
# CODE
# =============================================================================

# Tidy dependencies
code-dep-tidy:
    go mod tidy

# Update dependencies
code-dep-update:
    go get -u ./...
    go mod tidy

# Format code
code-fmt:
    gofmt -w .
    ./bin/goimports -w .

# Check formatting
code-fmt-check:
    gofmt -l .
    ./bin/goimports -l .

# Run all CI checks locally (fmt, lint, build, test)
code-check:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "==> gofmt"
    output=$(gofmt -l .)
    if [ -n "$output" ]; then echo "gofmt: $output"; exit 1; fi
    echo "==> goimports"
    output=$(./bin/goimports -l .)
    if [ -n "$output" ]; then echo "goimports: $output"; exit 1; fi
    echo "==> golangci-lint"
    ./bin/golangci-lint run ./...
    echo "==> build"
    go build ./...
    echo "==> test"
    go test ./...
    echo "==> test (race)"
    go test -race ./...
    echo "all checks passed"

# Run linter
code-lint:
    ./bin/golangci-lint run ./...

# Run linter with auto-fix
code-lint-fix:
    ./bin/golangci-lint run --fix ./...

# Run all tests
code-test:
    go test ./...

# Run tests with coverage
code-test-cover:
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html

# Run tests with race detector
code-test-race:
    go test -race ./...
