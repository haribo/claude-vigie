# Default: list all commands
default:
    @just --list

# Configure git hooks for local development
[group("dev")]
dev-setup:
    git config core.hooksPath .githooks
    @echo "git hooks configured (.githooks/)"

# Install dev tools (golangci-lint v2, goimports) into ./bin
[group("tool")]
tool-install:
    @mkdir -p bin
    GOBIN=$(pwd)/bin go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4
    GOBIN=$(pwd)/bin go install golang.org/x/tools/cmd/goimports@latest
    GOBIN=$(pwd)/bin go install golang.org/x/vuln/cmd/govulncheck@latest

# The dev recipes run the current source against a throwaway local server,
# isolated from any installed production client via VIGIE_CONFIG (never touches
# ~/.config). They track their pid in .dev/ and kill only that pid on restart —
# never by binary name, so a production watcher is never hit.

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
[group("dev")]
dev-config:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p "{{dev_dir}}"
    printf 'server_url = "{{dev_url}}"\ntoken = "{{dev_token}}"\nmachine = "%s-dev"\n' "$(hostname)" > "{{dev_config}}"
    echo "wrote {{dev_config}}"

# Stop the background dev server and watcher.
[group("dev")]
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
[group("dev")]
[doc("Add a dev hook leg alongside production")]
dev-hooks-install: dev-config
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p "{{dev_bin}}"
    go build -o "{{dev_bin}}/vigie" ./cmd/vigie
    VIGIE_CONFIG="{{dev_config}}" "{{dev_bin}}/vigie" hooks install

# Remove the dev hook leg (production hooks are never touched).
[group("dev")]
dev-hooks-uninstall:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p "{{dev_bin}}"
    [ -x "{{dev_bin}}/vigie" ] || go build -o "{{dev_bin}}/vigie" ./cmd/vigie
    VIGIE_CONFIG="{{dev_config}}" "{{dev_bin}}/vigie" hooks uninstall

# Start the source server in the background (rebuild + restart, throwaway db).
[group("dev")]
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

# Run the current-source TUI in the foreground, pointed at the dev server.
[group("dev")]
dev-tui: dev-config
    VIGIE_CONFIG="{{dev_config}}" go run ./cmd/vigie tui

# Start the current-source watcher in the background, pointed at the dev server.
[group("dev")]
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
# Run all CI checks locally (fmt, lint, build, test)
[group("code")]
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
    echo "==> javascript tests"
    just code-test-js
    echo "==> govulncheck"
    just code-vuln
    echo "all checks passed"

# Tidy dependencies
[group("code")]
code-dep-tidy:
    go mod tidy

# Update dependencies
[group("code")]
code-dep-update:
    go get -u ./...
    go mod tidy

# Format code
[group("code")]
code-fmt:
    gofmt -w .
    ./bin/goimports -w .

# Check formatting
[group("code")]
code-fmt-check:
    gofmt -l .
    ./bin/goimports -l .

# Run linter
[group("code")]
code-lint:
    ./bin/golangci-lint run ./...

# Run linter with auto-fix
[group("code")]
code-lint-fix:
    ./bin/golangci-lint run --fix ./...

# Run all tests
[group("code")]
code-test:
    go test ./...

# Run tests with coverage
[group("code")]
code-test-cover:
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html

# Run the JavaScript tests (dashboard + GNOME indicator)
#
# The two shipped artefacts cannot be imported outside their runtime —
# extension.js pulls in `gi://` and GNOME Shell resources, app.js drives the DOM —
# so the rules worth checking live in a sibling `lib.js` that both the artefact and
# these tests import. No package.json, no dependency: node's built-in runner (#430).
[group("code")]
[doc("Run the JavaScript tests (dashboard + GNOME indicator)")]
code-test-js:
    node --test test/js/dashboard.test.mjs test/js/gnome.test.mjs test/js/boot.test.mjs

# Run tests with race detector
[group("code")]
code-test-race:
    go test -race ./...

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

[group("code")]
[doc("Scan for known vulnerabilities, standard library included")]
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

# Regenerate the README animation from its template
#
# The asset was once produced by a script nobody committed, so it could not be
# rebuilt and nothing detected it going stale (#450). Edit
# internal/animation/template.svg or its palettes, run this, and commit the four
# files it writes — a test compares them against a fresh render, so forgetting
# fails the build.
[group("docs")]
[doc("Regenerate the README animation from its template")]
docs-animation:
    go run ./tools/animation
