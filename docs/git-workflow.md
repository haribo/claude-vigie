# Git workflow

See also: [git-commits.md](git-commits.md) for commit conventions, [git-issues.md](git-issues.md) for issue conventions.

## Branches

| Branch | Role |
|--------|------|
| `main` | Stable, tagged releases |
| `develop` | Integration branch — the default branch, where features land |

Both branches are permanent — never push directly, always via PR.

## Issue-first workflow

Every change starts with a GitHub issue, except trivial changes (typo, formatting, dependency bump) where the PR alone suffices.

- The issue describes the **what/why** — the PR describes the **how**
- The branch name includes the issue number for traceability
- The PR body references the issue with `Closes #N` to auto-close on merge

```
issue #12 → branch feat/12-web-dashboard → PR "Closes #12" → squash merge
```

## Feature workflow

```bash
# 1. Create issue
/gh-issue

# 2. Create branch from develop — include issue number
git checkout -b feat/12-web-dashboard develop

# 3. Work, commit
/git-commit

# 4. Review branch before opening PR
/git-review

# 5. Rebase on develop before opening PR
git fetch origin && git rebase origin/develop

# 6. Open PR — MUST target develop, reference the issue
/gh-pr-create

# 7. Wait for CI to pass
gh pr checks

# 8. Squash merge — NEVER use --merge on feature PRs
/gh-merge-develop
```

## Release workflow

`CHANGELOG.md` is the **single source of truth** for release notes
([Keep a Changelog](https://keepachangelog.com/)). Every user-facing PR adds a
line under `## [Unreleased]`. On release, that section is rolled into a versioned
one, and the GitHub Release body is a **mirror** of it — never a second narrative,
never goreleaser's commit dump (its `changelog:` is disabled for this reason).

Use the **`/release`** skill: it runs the flow below and **stops for explicit
human approval before the `develop`→`main` merge, and again before the tag push**
— a release is never autonomous.

```bash
# 1. develop is green, and CHANGELOG.md has entries under [Unreleased]
# 2. Roll [Unreleased] → [X.Y.Z] - YYYY-MM-DD in CHANGELOG.md; commit to develop
# 3. Open PR develop → main (only when develop is validated)
gh pr create --base main --head develop
# 4. Wait for CI                            gh pr checks
#    ⛔ STOP — explicit human approval before merging
# 5. Merge commit — NEVER squash a release PR
gh pr merge --merge
#    ⛔ STOP — explicit human approval before tagging
# 6. Tag → goreleaser publishes the binaries
git tag vX.Y.Z && git push origin vX.Y.Z
# 7. Set the Release body from the CHANGELOG [X.Y.Z] section (the mirror)
gh release edit vX.Y.Z --notes-file <section>
```

## Merge strategy

| Target | Strategy | Command |
|--------|----------|---------|
| Feature → `develop` | **Squash** | `/gh-merge-develop` |
| `develop` → `main` | **Merge commit** | `gh pr merge --merge` |

**NEVER merge a feature PR with `--merge` — always squash via `/gh-merge-develop`.**
**NEVER target `main` with a feature PR — always target `develop`.**

## Rules

- Never commit or push directly to `main` or `develop` — always via a feature branch and PR
- One logical change per PR — split unrelated work into separate PRs
- Keep feature branches short-lived (days, not weeks)
- Rebase on `develop` before opening PR to avoid merge conflicts

## Protected branches

`main` and `develop` are protected at three levels — defense in depth:

| Level | Mechanism |
|-------|-----------|
| Local | `.githooks/pre-commit` refuses any commit made directly on `main` or `develop` |
| Server | GitHub ruleset: require a PR to merge, block force-push, block deletion |
| Process | This doc + `CLAUDE.md` + the `/git-commit` command |

The only unavoidable exception is the **repository's root commit**: a PR requires
a base branch that already exists on the remote, so the very first commit of the
repo (`.gitignore`) is pushed directly to `develop`. Everything after that goes
through a PR.

## Branch naming

```
feat/12-short-description
fix/34-short-description
refactor/56-short-description
docs/78-short-description
chore/short-description
```

Prefix matches commit type. Include issue number after the slash. Use kebab-case.
May omit issue number for trivial `chore`/`style` changes without an issue.
