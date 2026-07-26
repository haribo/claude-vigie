# Branch Review

Deep review of current branch changes against project conventions. Local only — no GitHub interaction.

## Execution

This review **must** run in an isolated subagent (Task tool) to avoid confirmation bias from the current conversation context. Launch a `general-purpose` subagent with the full instructions below as its prompt.

## Instructions

### 1. Validate branch

- Run `git branch --show-current`
- If on `main` or `develop`: **refuse** — must be on a feature branch
- Run `git log --oneline develop..HEAD` — if no commits ahead: **refuse** — nothing to review

### 2. Read sources of truth

Read these living docs before reviewing (conventions may evolve):

- `docs/code.md` — code conventions (architecture, DI, error handling, comments)
- `docs/architecture.md` — component boundaries and data flow
- `docs/git-commits.md` — commit message format

### 3. Collect change data

Run in parallel:

- `git log --oneline develop..HEAD`
- `git diff develop...HEAD`
- `git diff develop...HEAD --name-only`

### 4. Read changed files in full

- Extract all changed `.go` files from the diff
- Read each file **in full** (not just the diff) — needed for structural rules like import boundaries, file organization, error handling patterns
- For each changed source file (excluding generated code), check if a corresponding `_test.go` file exists

### 5. First pass — identify findings

Review against every category below. For each finding, record the file:line reference and the rule violated.

| Category | Key checks | Source |
|----------|-----------|--------|
| Commits | format, max 72 chars, imperative, lowercase, no period, no AI refs | `docs/git-commits.md` |
| Scope | one logical change, no opportunistic refactoring, no unrelated changes, no TODOs/FIXMEs without linked issue | `docs/git-workflow.md` |
| Architecture | package boundaries respected, one struct per file where it has methods, dependencies injected via constructors | `docs/code.md`, `docs/architecture.md` |
| Dependency injection | no hidden global state; `time.Now`, `os.Getenv`, network clients injected, not called directly in business logic | `docs/code.md` |
| Error handling | wrap with context (`fmt.Errorf("...: %w", err)`), no log-and-return, no swallowed errors | `docs/code.md` |
| Comments | WHY not HOW, no restating code | `docs/code.md` |
| Security | missing auth on endpoints that push/read fleet data, secrets in logs/errors, unvalidated input | `docs/code.md` |
| Tests | new package/behavior without a corresponding test, modified behavior without updated test | project rules |

### 6. Second pass — validate findings

For **every** finding from the first pass:

1. Re-read the actual code at the flagged location
2. Apply false positive rules (see below)
3. Mark as **CONFIRMED** or **INVALIDATED** with justification
4. Drop all INVALIDATED findings from the final report

### 7. False positive rules

Do NOT flag:

- **Generated code** (files with `// Code generated` header): skip entirely
- **Test files** (`_test.go`): relaxed rules on concrete dependencies and error wrapping in test helpers
- **DTOs/types in a shared `types.go`**: multiple struct definitions are expected — the one-struct-per-file rule applies to structs with methods

### 8. Classify severity

| Severity | Criteria |
|----------|----------|
| **Critical** | Security issues (missing auth, secret leaks), broken architecture (package boundary violations) |
| **High** | Hidden global state in business logic, missing tests for new behavior, missing error wrapping |
| **Medium** | Comment quality, file organization, missing test updates for modified behavior, scope violations |
| **Low** | Minor naming issues, commit message formatting |

### 9. Determine verdict

| Condition | Verdict |
|-----------|---------|
| Any Critical or High finding | CHANGES REQUESTED |
| 3+ Medium findings | CHANGES REQUESTED |
| 1–2 Medium or only Low findings | APPROVED (with notes) |
| Zero findings | APPROVED |

### 10. Present report

Show the full report and verdict to the user.

```
## Branch Review

### Verdict: <CHANGES REQUESTED / APPROVED>

### Findings

#### Critical
- **[Category]** `file:line` — description
(or "None")

#### High
...

#### Medium
...

#### Low
...

### Tests
<coverage status — new files with/without corresponding _test.go>

### Commits
<convention compliance status>
```

Stop here. The user fixes issues and runs `/git-review` again, or proceeds with `/gh-pr-create`.
