# Pull Request

Run local checks, push, and create a PR targeting `develop`.

Accepts an optional argument: PR title. If not provided, generate one from commits.

## Instructions

### 1. Validate branch

- Run `git branch --show-current`
- If on `main` or `develop`: **refuse** — must be on a feature branch

### 2. Resolve issue reference

- Extract issue number from the branch name: convention is `type/NUMBER-description` (e.g., `feat/12-web-dashboard` → `#12`)
- Parse the number immediately after the first `/`
- If no issue number found and the PR title does NOT start with `chore` or `style`: **refuse** — ask the user to provide the issue number
- If the PR title starts with `chore` or `style`: skip — no issue reference required (matches CI exemption)

### 3. Run local checks

- Run `just code-check`
- If it fails: report the error and **stop** — do not push broken code

### 4. Prepare PR content

Run in parallel:
- `git log --oneline develop..HEAD` to see all commits
- `git diff develop...HEAD --stat` to see changed files

Analyze all commits and draft:
- **Title**: `type(scope): description` format (under 70 chars), no `(#N)` suffix — it is appended on squash merge. Use argument if provided.
- **Body**: summary bullets + test plan + `Closes #N` (if issue number was resolved in step 2)

### 5. Push and create PR

- Push with `git push -u origin <branch>`
- Create PR:

```
gh pr create --title "<title>" --body "$(cat <<'EOF'
## Summary
<1-3 bullet points>

## Test plan
<bulleted checklist>

Closes #<N>
EOF
)"
```

Omit the `Closes #<N>` line for `chore`/`style` PRs (no issue reference).

### 6. Output

Return the PR URL.
