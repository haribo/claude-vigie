# Commit conventions

See also: [git-workflow.md](git-workflow.md) for branching and PR rules.

## Format

```
<type>(<scope>): <description>
<type>(<scope>)!: <description>   ← breaking change
```

## Types

`feat` | `fix` | `docs` | `style` | `refactor` | `perf` | `test` | `chore` | `ci` | `build`

## Scope

Recommended on all commits. Matches the area of change (package or component).

Examples: `server`, `report`, `tui`, `web`, `config`, `cli`, `store`, `github`, `justfile`

May be omitted for generic `style` or `chore` that span the whole project.

## Breaking changes

Append `!` after the scope to flag a breaking change:

```
feat(report)!: change hook payload format
```

## Squash merge commits

When a feature PR is squash-merged into `develop`, GitHub auto-appends the PR number:

```
type(scope): description (#PR)
```

The PR title must follow `type(scope): description` — without `(#PR)`, GitHub adds it on merge.

## Rules

1. Single line only — no body, no footer
2. Max 72 characters (excluding auto-appended `(#PR)` suffix)
3. Imperative present tense ("add" not "added")
4. No capital letter, no period
5. No AI references or promotional content
