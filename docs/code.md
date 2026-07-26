# Code guidelines

## Checklist

After modifying Go code:

1. **Tests**: write/update tests, run `just code-test`
2. **Linter**: run `just code-lint`
3. **Docs**: update documentation if behavior or public API changed

## Comments

**Comments should explain WHY, never HOW.** If code needs explanation, simplify it.

### Good

```go
// Read the transcript tail instead of the whole file: sessions grow to
// megabytes and we only need the latest usage counters.
```

### Bad

```go
// Loop over the lines and parse each one
```

### When to write comments

- Business logic rationale
- Non-obvious constraints or invariants
- References to external requirements (Claude Code hook contract, specs, tickets)
- Workarounds for known bugs in dependencies

### When NOT to write comments

- Describing what the code does (the code already does that)
- Restating function/variable names in prose
- Explaining language features or standard library usage

## Architecture

The binary is a single executable with several subcommands. Each concern lives
in its own package under `internal/`.

| Package | Role |
|---------|------|
| `internal/cli` | Command-line dispatch; one file per subcommand |
| `internal/config` | Load/save the shared per-machine client config (XDG) |
| `internal/version` | Build metadata injected via ldflags |
| `internal/store` | SQLite persistence: sessions, events, usage |
| `internal/server` | HTTP API, auth, SSE, embedded web dashboard |
| `internal/report` | Reporter: parse hook payloads + transcripts, POST to the server |
| `internal/tui` | Terminal dashboard client (Bubble Tea) |
| `web/` | Static dashboard assets, embedded via `embed.FS` |

### Boundaries

- `cli` wires subcommands to the packages above; it holds no business logic
- `server` and `tui` depend on `store` / HTTP types, never on each other
- `config` and `version` are leaves — they import nothing from the project
- Keep transport (HTTP handlers) separate from persistence (`store`)

### File organization

- One file = one struct and its methods (e.g. `server.go`, `sqlite.go`)
- Shared DTOs/types may live together in a `types.go`
- One responsibility per function, one purpose per package

## Dependency injection

Accept interfaces, return structs. Inject dependencies via constructors.

Do not reach for hidden global state in business logic. Inject, don't call
directly:

| Call | Replace with |
|------|--------------|
| `time.Now()` | an injected `Clock` / `func() time.Time` |
| `os.Getenv()` | a config struct |
| `http.DefaultClient` | an injected `*http.Client` |

## Error handling

Errors are values. Wrap with context, don't log and return.

```go
// Good: wrap with context
if err != nil {
    return fmt.Errorf("reading transcript: %w", err)
}
```

Do not log an error and also return it — it gets handled twice. Log at the
boundary where the error stops propagating.

## Factorization

Don't duplicate logic — extract when the same pattern appears twice. But don't
abstract prematurely: three similar lines are better than a premature helper.
