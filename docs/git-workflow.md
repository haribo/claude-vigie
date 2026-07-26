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

```bash
# 1. Open PR develop → main (only when develop is validated)
gh pr create --base main --head develop

# 2. Wait for CI to pass
gh pr checks

# 3. Merge commit — NEVER squash release PRs
gh pr merge --merge

# 4. Tag the release
git tag vX.Y.Z && git push origin vX.Y.Z
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
