# Issue conventions

See also: [git-commits.md](git-commits.md) for commit conventions, [git-workflow.md](git-workflow.md) for branching and PR rules.

## Title

```
<imperative description>
```

The title is a pure description — the type is carried by labels, not the title.

## Rules

1. Imperative present tense ("add" not "added")
2. Lowercase, no period
3. Max 72 characters
4. Descriptive and concise — the title should stand alone without needing context

## Labels

Use prefixed labels to categorize issues. The title must not duplicate label information.

| Label | Usage |
|-------|-------|
| `type: bug` | defect or malfunction |
| `type: feature` | new feature or improvement |
| `type: chore` | CI, tooling, maintenance, cleanup |
| `type: docs` | documentation change |
| `priority: critical` | requires immediate attention |
| `state: wontfix` | will not be worked on |
| `state: duplicate` | already exists |
| `state: invalid` | not a valid issue |

## Examples

| Title | Labels |
|-------|--------|
| `serve subcommand exits without binding the listener` | `type: bug`, `priority: critical` |
| `store session events in sqlite` | `type: feature` |
| `add govulncheck job to ci` | `type: chore` |
