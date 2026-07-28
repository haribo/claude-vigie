# Releases & snapshots

Two distinct channels. **They must not be confused.**

| | Snapshot | Release |
|---|----------|---------|
| What | A build of the current `develop` state | A SemVer version |
| Stability | Unstable, mutable | Stable |
| Source branch | `develop` | `main` |
| Tag | rolling `snapshot` (overwritten each run) | `vX.Y.Z` (immutable) |
| Trigger | manual `workflow_dispatch` | pushing a `v*` tag |
| Pipeline | `.github/workflows/release-snapshot.yml` | `.github/workflows/release.yaml` (goreleaser) |
| Versioned? | **No** | Yes, with a CHANGELOG |

A snapshot is **not a version** and is never presented as stable. Cutting a
SemVer release is a deliberate, manual decision — see [below](#semver-releases).

## Snapshots

A snapshot lets you install and test the current `develop` without cutting a
version. Each run **overwrites** the previous one.

**Produce one:** GitHub → Actions → **Snapshot** → *Run workflow*. It builds from
`develop`, gates on `go vet` + `go test`, and publishes:

- a GitHub **pre-release** under the rolling `snapshot` tag, with the Linux
  binaries `claude-fleet` and `claude-fleetd` (amd64, arm64);
- the server container image `ghcr.io/haribo/claude-fleetd:snapshot`.

**Build stamp.** Every snapshot binary is stamped `develop-<date>-<sha7>`,
injected into `internal/version` and visible via:

```bash
claude-fleet version      # claude-fleet develop-20260728-ab74e54 (...)
```

**Install:**

```bash
# binaries — from the "snapshot" pre-release on the Releases page
curl -sSL -o claude-fleet \
  https://github.com/haribo/claude-fleet/releases/download/snapshot/claude-fleet-linux-amd64-<stamp>

# server image
docker pull ghcr.io/haribo/claude-fleetd:snapshot
docker run --rm -p 8080:8080 -e FLEET_TOKEN=<token> \
  -v claude-fleet-data:/data ghcr.io/haribo/claude-fleetd:snapshot
```

The image runs as a non-root user; the SQLite database lives at `/data` (mount a
volume to persist it). The token comes from `FLEET_TOKEN`; override the command
to change `--addr` or `--db`.

## SemVer releases

Reserved for when `develop` has been validated on a real server. The flow (see
[docs/git-workflow.md](git-workflow.md)):

1. PR `develop → main` (merge commit, never squash).
2. Tag `git tag vX.Y.Z && git push origin vX.Y.Z`.
3. The `v*` tag triggers goreleaser, which publishes a stable GitHub Release with
   cross-platform archives, checksums, and a grouped changelog.

Snapshots never touch `main`.
