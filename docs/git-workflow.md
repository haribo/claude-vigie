# Git workflow

See also: [git-commits.md](git-commits.md) for commit conventions, [git-issues.md](git-issues.md) for issue conventions.

## Branches

| Branch | Role |
|--------|------|
| `main` | Stable, tagged releases — **the default branch** |
| `develop` | Integration branch, where features land |

Both branches are permanent — never push directly, always via PR.

**Why `main` is the default.** It is what a visitor lands on, and it is the only
branch that describes something they can download: `main` matches the latest tag
and the published binaries. A README on `develop` describes a build that exists
nowhere. Both have a lag — a doc correction stays invisible on the landing page
until the next release — and only one of them is consistent with what is online.

The cost is real and is paid deliberately: **the safe base for a feature PR
stopped being the automatic one.** `gh pr create` and the GitHub UI now offer
`main` first. Two things answer that, and they are one decision with the default
itself:

- `.claude/commands/gh-pr-create.md` passes `--base develop` **explicitly**;
- `.github/workflows/pr-base.yaml` refuses a PR that targets `main` from anything
  but `develop`, and is a required check — so the mistake is blocked, not merely
  reported.

## Issue-first workflow

Every change starts with a GitHub issue, except trivial changes (typo, formatting, dependency bump) where the PR alone suffices.

- The issue describes the **what/why** — the PR describes the **how**
- The branch name includes the issue number for traceability
- The PR body references the issue with `Closes #N`. It does **not** auto-close:
  GitHub only does that when a PR merges into the default branch, and that is
  `main` — a feature PR merges into `develop`. `/gh-merge-develop` closes the
  issue itself, naming the merge commit. `Part of #N` deliberately closes nothing

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
line under `## [Unreleased]` — one or two sentences and the issue number, no
more: see [changelog.md](changelog.md), which a test enforces. On release, that section is rolled into a versioned
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
| Server | GitHub ruleset `protect main and develop`: require a PR, block force-push, block deletion, require status checks |
| Process | This doc + `CLAUDE.md` + the `/git-commit` command |

The ruleset covers **both** branches with one list of required checks, which is
why `pr-base.yaml` also runs on PRs to `develop` and passes there immediately: a
required check that never reported on a develop PR would leave every one of them
waiting for a status that is not coming.

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
