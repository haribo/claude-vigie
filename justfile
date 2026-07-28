# Application version (overridden by git tag on release)
version := "0.0.0"
commit := `git rev-parse --short HEAD 2>/dev/null || echo none`
build_time := `date -u +%Y-%m-%dT%H:%M:%SZ`
ldflags := "-X github.com/haribo/claude-fleet/internal/version.Version=" + version + " -X github.com/haribo/claude-fleet/internal/version.Commit=" + commit + " -X github.com/haribo/claude-fleet/internal/version.BuildTime=" + build_time

# Fleet self-host paths (local dogfooding)
home_bin := env_var("HOME") / ".local/bin"
fleet_data := env_var("HOME") / ".local/share/claude-fleet"
fleet_db := fleet_data / "fleet.db"
fleet_host := "127.0.0.1"
fleet_port := "8080"
fleet_addr := fleet_host + ":" + fleet_port
fleet_url := "http://" + fleet_host + ":" + fleet_port

# Default: list all commands
default:
    @just --list

# =============================================================================
# DEV
# =============================================================================

# Set up local dev environment (git hooks)
dev-setup:
    git config core.hooksPath .githooks
    @echo "git hooks configured (.githooks/)"

# =============================================================================
# APP
# =============================================================================

# Build both binaries into ./bin
app-build:
    go build -ldflags "{{ldflags}}" -o bin/claude-fleet ./cmd/claude-fleet
    go build -ldflags "{{ldflags}}" -o bin/claude-fleetd ./cmd/claude-fleetd

# Clean build artifacts
app-clean:
    rm -rf bin/

# Build and run the server in the foreground
app-serve: app-build
    mkdir -p {{fleet_data}}
    ./bin/claude-fleetd serve --addr {{fleet_addr}} --db {{fleet_db}}

# Build and run the terminal client
app-tui: app-build
    ./bin/claude-fleet tui

# Build and run the watcher in the foreground
app-watch: app-build
    ./bin/claude-fleet watch

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

# =============================================================================
# TOOLS
# =============================================================================

# Install dev tools (golangci-lint v2, goimports) into ./bin
tool-install:
    @mkdir -p bin
    GOBIN=$(pwd)/bin go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4
    GOBIN=$(pwd)/bin go install golang.org/x/tools/cmd/goimports@latest

# =============================================================================
# FLEET (self-host / dogfood)
# =============================================================================

# Install both binaries into ~/.local/bin (must be on your PATH)
install: app-build
    mkdir -p {{home_bin}}
    # Atomic replace via mv so a running service (holding the old inode) does
    # not fail with "Text file busy".
    cp -f bin/claude-fleet {{home_bin}}/.claude-fleet.tmp && mv -f {{home_bin}}/.claude-fleet.tmp {{home_bin}}/claude-fleet
    cp -f bin/claude-fleetd {{home_bin}}/.claude-fleetd.tmp && mv -f {{home_bin}}/.claude-fleetd.tmp {{home_bin}}/claude-fleetd
    # Restart running services so the new binary takes effect (a copy alone
    # leaves the old binary loaded). try-restart skips stopped services.
    -@systemctl --user try-restart claude-fleetd.service claude-fleet-watch.service 2>/dev/null
    @echo "installed claude-fleet + claude-fleetd into {{home_bin}} (restarted any running services)"

# Print the local fleet auth token
fleet-token: app-build
    @./bin/claude-fleetd token --db {{fleet_db}}

# Connect this machine to the local fleet (writes config + Claude Code hooks)
fleet-connect: app-build
    ./bin/claude-fleet init --server {{fleet_url}} --token "$(./bin/claude-fleetd token --db {{fleet_db}})" --machine "$(hostname)"

# Disconnect this machine (remove our hooks)
fleet-disconnect: app-build
    ./bin/claude-fleet init --uninstall

# Install & start the SERVER as a systemd --user service (host only, Linux)
fleet-server-install: install
    scripts/install-systemd.sh server {{home_bin}} {{fleet_addr}} {{fleet_db}}

# Stop & remove the server service
fleet-server-uninstall:
    -systemctl --user disable --now claude-fleetd.service
    rm -f ~/.config/systemd/user/claude-fleetd.service
    systemctl --user daemon-reload
    @echo "removed claude-fleetd.service"

# Install & start the WATCHER as a systemd --user service (every machine, Linux)
fleet-watch-install: install
    scripts/install-systemd.sh watch {{home_bin}}

# Stop & remove the watcher service
fleet-watch-uninstall:
    -systemctl --user disable --now claude-fleet-watch.service
    rm -f ~/.config/systemd/user/claude-fleet-watch.service
    systemctl --user daemon-reload
    @echo "removed claude-fleet-watch.service"
