# Develop Merge

Merge a feature PR into `develop` following project conventions with strict validation.

Accepts an optional argument: PR number or branch name. If not provided, detect from current branch.

## Instructions

### 1. Resolve target PR

- If argument is a PR number (`#123` or `123`): use it directly
- If argument is a branch name: find the open PR for that branch targeting `develop`
- If no argument: detect current branch, find its open PR targeting `develop`
- If no PR found: **refuse** — no PR, no merge

### 2. Collect PR information

Run all of these in parallel:
- `gh pr view <number> --json number,title,state,baseRefName,headRefName,mergeable,reviews,statusCheckRollup,commits,body,labels`
- `gh pr checks <number>`
- `gh pr diff <number>`

### 3. Validate merge readiness — fail on first violation

Perform every check below. Collect **all** violations, then report them together. Do NOT stop at the first failure.

| # | Check | Rule |
|---|-------|------|
| 1 | PR state | Must be `OPEN` |
| 2 | Base branch | Must be `develop` — refuse any other target |
| 3 | Mergeability | `mergeable` must be `MERGEABLE` — if `CONFLICTING`, demand rebase |
| 4 | CI status | Every required check must be `pass` or `success` — no pending, no failures |
| 5 | Reviews | Zero `CHANGES_REQUESTED` — if any, report and stop |
| 6 | Diff review | Read the full diff — flag anything suspicious: debug prints, TODO/FIXME, hardcoded secrets, commented-out code, unrelated changes |

If **any** check fails: report all violations in a structured summary and **stop**. Do not proceed.

### 4. Craft squash commit message

- Read `docs/git-commits.md` for format rules
- Analyze the PR title, body, and all individual commit messages
- Determine the correct `type(scope):` prefix from the actual changes (not blindly from PR title)
- Write a single-line commit message (max 72 chars excluding the (#N) suffix, imperative, no capital, no period)
- Append " (#<PR number>)" to the commit message — required because --subject overrides GitHub's auto-append
- If the PR contains breaking changes: use `type(scope)!:` format
- Present the proposed commit message to the user for approval before merging

### 5. Merge

- Execute: `gh pr merge <number> --squash --delete-branch --subject "<approved message>" --body ""`
- If merge fails: report the error, do not retry

### 6. Close the issues the PR resolves

GitHub auto-closes an issue only when the PR merges into the **default branch**,
which is `main`. A feature PR merges into `develop`, so `Closes #N` links the two
and closes nothing — the issue would stay open until the next release.

- Extract every closing reference from the PR body, the same keywords
  `.github/workflows/pr-issue-check.yaml` accepts:
  `(close[sd]?|fix(e[sd])?|resolve[sd]?)\s+#[0-9]+`, case-insensitive.
- **`Part of #N` is not one of them** and must not close anything: a PR that
  advances an issue without finishing it says so on purpose.
- Close each with the merge commit, so the issue records where it was resolved:

```bash
gh issue close <N> --comment "Resolved by <sha> on develop (#<PR>). It reaches a release with the next tag."
```

- If the body carries no closing reference — a `chore` or `style` PR, which the
  CI check exempts — skip this step silently.
- Report which issues were closed; if one was already closed, say so rather than
  treating the command's success as proof it did something.

### 7. Local cleanup

- `git checkout develop`
- `git pull origin develop`
- If the feature branch still exists locally: `git branch -d <branch>`
- Confirm merge success with `git log --oneline -1`

## Output format

Report a summary:

```
PR #<number> merged into develop
  <commit hash> <commit message>
  Branch <branch> deleted (remote + local)
  Closed #<N> (or: no closing reference in the body)
```
