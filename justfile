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
    GOBIN=$(pwd)/bin go install golang.org/x/vuln/cmd/govulncheck@latest

# =============================================================================
# DEV RUN — run the current source against a throwaway local server, isolated
# from any installed production client via VIGIE_CONFIG (never touches
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
    go build -o "{{dev_bin}}/vigied" ./cmd/vigied
    pidf="{{dev_dir}}/server.pid"
    [ -f "$pidf" ] && kill "$(cat "$pidf")" 2>/dev/null || true
    nohup "{{dev_bin}}/vigied" serve --addr {{dev_addr}} --token {{dev_token}} --db "{{dev_db}}" --session-retention 0 --metrics-addr 0.0.0.0:9464 > "{{dev_dir}}/server.log" 2>&1 &
    echo $! > "$pidf"
    echo "dev server → {{dev_url}} (pid $(cat "$pidf"), logs {{dev_dir}}/server.log)"

# Start the current-source watcher in the background, pointed at the dev server.
dev-watcher: dev-config
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p "{{dev_bin}}"
    go build -o "{{dev_bin}}/vigie" ./cmd/vigie
    pidf="{{dev_dir}}/watcher.pid"
    [ -f "$pidf" ] && kill "$(cat "$pidf")" 2>/dev/null || true
    VIGIE_CONFIG="{{dev_config}}" nohup "{{dev_bin}}/vigie" watch > "{{dev_dir}}/watcher.log" 2>&1 &
    echo $! > "$pidf"
    echo "dev watcher → {{dev_url}} (pid $(cat "$pidf"), logs {{dev_dir}}/watcher.log)"

# Run the current-source TUI in the foreground, pointed at the dev server.
dev-tui: dev-config
    VIGIE_CONFIG="{{dev_config}}" go run ./cmd/vigie tui

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

# Add a dev hook leg — sessions report to the dev server too, alongside prod
# (fills waiting/active in dev). No-ops silently when the dev server is down.
dev-hooks-install: dev-config
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p "{{dev_bin}}"
    go build -o "{{dev_bin}}/vigie" ./cmd/vigie
    VIGIE_CONFIG="{{dev_config}}" "{{dev_bin}}/vigie" hooks install

# Remove the dev hook leg (production hooks are never touched).
dev-hooks-uninstall:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p "{{dev_bin}}"
    [ -x "{{dev_bin}}/vigie" ] || go build -o "{{dev_bin}}/vigie" ./cmd/vigie
    VIGIE_CONFIG="{{dev_config}}" "{{dev_bin}}/vigie" hooks uninstall

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
    echo "==> test (race)"
    go test -race ./...
    echo "==> govulncheck"
    just code-vuln
    echo "all checks passed"

# Scan for known vulnerabilities, standard library included
#
# govulncheck matches standard-library advisories against the toolchain's version
# string. A distribution-built Go reports a non-canonical one — Arch's is
# `go1.26.5-X:nodwarf5` — which matches no advisory, so every stdlib finding is
# dropped and the scan still prints "No vulnerabilities found". That silence is
# the defect this recipe removes (#426).
#
# GOTOOLCHAIN pins a canonical toolchain, which Go downloads once. It is a floor
# for the local pre-check, not the authority: CI resolves the newest patch release
# itself (`check-latest`, #425) and remains what gates a merge. Bump this when CI
# reports a stdlib advisory that the local scan misses.
vuln-toolchain := "go1.26.6"

code-vuln:
    #!/usr/bin/env bash
    set -euo pipefail
    export GOTOOLCHAIN="{{vuln-toolchain}}"
    # Assert the pin took effect. Anything but a canonical goX.Y.Z means stdlib
    # advisories were skipped, and a scan that cannot see them must say so rather
    # than report all-clear.
    version=$(./bin/govulncheck -version | sed -n 's/^Go: *//p' | head -1)
    if ! [[ "$version" =~ ^go[0-9]+(\.[0-9]+)*$ ]]; then
        echo "govulncheck ran under a non-canonical toolchain ($version):" >&2
        echo "standard-library advisories would be skipped silently. Refusing." >&2
        exit 1
    fi
    ./bin/govulncheck ./...

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
