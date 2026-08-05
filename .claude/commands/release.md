# Release

Cut a release: roll the changelog, merge `develop` → `main`, tag, and mirror the
`CHANGELOG.md` section into the GitHub Release notes.

Accepts an optional argument: the version `X.Y.Z`. If not provided, propose one
from the `[Unreleased]` entries (semver: breaking → major, feat → minor, fix →
patch) and confirm with the user.

**A release is NEVER autonomous.** This skill has TWO hard stops — before the
`develop`→`main` merge and before the tag push. At each, present the exact command
and **wait for the human to run it** (`! <command>`); never merge to `main` or push
a tag yourself.

## Instructions

### 1. Preconditions — stop on any failure

- On `develop`, clean working tree, synced with `origin/develop`.
- `develop` CI is green (`gh run list --branch develop --event push --limit 1`).
- `CHANGELOG.md` has content under `## [Unreleased]` (nothing to release otherwise).
- Resolve `X.Y.Z` (argument, or proposed from the entries and confirmed).

### 2. Roll the changelog (a normal PR to develop)

- On a branch `chore/changelog-vX.Y.Z`: in `CHANGELOG.md`, rename `## [Unreleased]`
  to `## [X.Y.Z] - YYYY-MM-DD` (today), add a fresh empty `## [Unreleased]` above
  it, and update the link footer (`[Unreleased]` compare → `vX.Y.Z...HEAD`, add
  `[X.Y.Z]` → the release tag URL).
- Commit via `/git-commit` (`chore(changelog): roll vX.Y.Z`), open a PR with
  `/gh-pr-create`, and merge it with `/gh-merge-develop`. `develop` now carries the
  rolled changelog.

### 3. Open the release PR

- `gh pr create --base main --head develop --title "chore(release): cut vX.Y.Z"`
  with a body summarizing the CHANGELOG section.
- Wait for CI: `gh pr checks <N> --watch`. All required checks must pass.

### 4. ⛔ GATE 1 — human approval before merge

- Present the PR, the CHANGELOG section, and the **exact merge command**:
  `! gh pr merge <N> --merge --subject "chore(release): cut vX.Y.Z (#N)" --body ""`
  *(merge commit — NEVER squash a release PR; `--subject` keeps the merge commit a
  conventional commit instead of GitHub's default "Merge pull request …")*.
- **Stop. Do not merge yourself.** Continue only after the human has merged.

### 5. ⛔ GATE 2 — human approval before tag

- Fetch main: `git fetch origin`.
- Present the **exact tag command** for the human to run:
  `! git tag -a vX.Y.Z origin/main -m "vX.Y.Z" && git push origin vX.Y.Z`.
- **Stop. Do not tag or push yourself.** The tag push triggers goreleaser.

### 6. Set the Release notes (the mirror)

- After the human confirms the tag is pushed, wait for goreleaser to publish the
  release (`gh run watch`).
- Extract the `## [X.Y.Z]` section from `CHANGELOG.md` (heading excluded) to a
  file and set it as the body: `gh release edit vX.Y.Z --notes-file <file>`.
- Confirm the release body matches the CHANGELOG section (the mirror rule).

## Output

Report: the version, the changelog-roll PR, the release PR, the tag, and that the
Release notes mirror `CHANGELOG.md [X.Y.Z]`.
