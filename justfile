# Application version (overridden by git tag on release)
version := "0.0.0"

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

# Build the binary into ./bin
app-build:
    go build -ldflags "-X github.com/haribo/claude-fleet/internal/version.Version={{version}} -X github.com/haribo/claude-fleet/internal/version.Commit=$(git rev-parse --short HEAD 2>/dev/null || echo none) -X github.com/haribo/claude-fleet/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o bin/claude-fleet ./cmd/claude-fleet

# Clean build artifacts
app-clean:
    rm -rf bin/

# Build and run the server
app-serve: app-build
    ./bin/claude-fleet serve

# Build and run the terminal client
app-tui: app-build
    ./bin/claude-fleet tui

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
