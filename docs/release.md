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
`develop`, gates on `go vet` + `go test`, and publishes a GitHub **pre-release**
under the rolling `snapshot` tag, with the Linux binaries `vigie` and
`vigied` (amd64, arm64).

**Build stamp.** Every snapshot binary is stamped `develop-<date>-<sha7>`,
injected into `internal/version` and visible via:

```bash
vigie version      # vigie develop-20260728-ab74e54 (...)
```

**Install:**

```bash
# from the "snapshot" pre-release on the Releases page
curl -sSL -o vigied \
  https://github.com/haribo/claude-vigie/releases/download/snapshot/vigied-linux-amd64-<stamp>
chmod +x vigied
```

Deployment is out of scope: vigie ships **binaries**. How you run
`vigied` (container, systemd, …), terminate TLS (a reverse proxy such as
Caddy), and expose it is the deployer's call. The clients speak whatever scheme
is in their `server_url`, so point them at your `https://` endpoint; the shared
token is only safe over TLS.

## SemVer releases

Reserved for when `develop` has been validated on a real server. The flow (see
[docs/git-workflow.md](git-workflow.md)):

1. PR `develop → main` (merge commit, never squash).
2. Tag `git tag vX.Y.Z && git push origin vX.Y.Z`.
3. The `v*` tag triggers goreleaser, which publishes a stable GitHub Release with
   cross-platform archives, checksums, and a grouped changelog.

Snapshots never touch `main`.
