# Code guidelines

## Checklist

After modifying Go code:

1. **Tests**: write/update tests, run `just code-test`
2. **Linter**: run `just code-lint`
3. **Docs**: update documentation if behavior or public API changed

## Testing

- **Regression-test-first for bug fixes.** Every bug fix starts with a test that
  *reproduces the bug*: it fails before the fix, passes after, and stays as a
  guard. Line coverage is not a substitute — the status functions were near 100%
  covered when the #201, #233 and #235 bugs slipped through. Name the case after
  the issue and reference the number in a comment (see `TestReconcileWatch`, whose
  cases are tagged `#190`/`#201`/`#233`), so the guard stays traceable.
- **Sequence and interleaving bugs need a replay test** over the real path, not
  just unit tests of each layer in isolation (see `reconcile_timeline_test.go`,
  #203) — that is where the reconciliation bugs actually lived.
- Prefer table-driven cases; the coverage gate (#223) is a floor, not the goal.

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

Two binaries share code through `internal/`. The split is enforced by imports:
the client never imports the server/store packages, so it never links them.

| Binary | Command dir | Entry package |
|--------|-------------|---------------|
| `vigie` (client) | `cmd/vigie` | `internal/client` |
| `vigied` (daemon) | `cmd/vigied` | `internal/daemon` |

| Package | Used by | Role |
|---------|---------|------|
| `internal/client` | client | Client command dispatch (init, hooks, report, call, watch, tui) |
| `internal/daemon` | daemon | Daemon command dispatch (serve) |
| `internal/report` | client | Parse hook payloads + transcripts, POST to the server |
| `internal/tui` | client | Terminal dashboard client (Bubble Tea) |
| `internal/server` | daemon | HTTP API, auth, SSE, embedded web dashboard |
| `internal/store` | daemon | SQLite persistence: sessions, events, usage |
| `internal/config` | both | Load/save the shared per-machine client config (XDG) |
| `internal/api` | both | Shared request/response types (client ↔ server) |
| `internal/version` | both | Build metadata injected via ldflags |
| `internal/web` | daemon | Static dashboard assets, embedded via `go:embed` and served at `GET /` |
| `internal/watch` | client | Scans transcripts, derives status, reports every session |
| `internal/transcript` | client | Parses a session transcript, incrementally |
| `internal/apiclient` | client | One authenticated GET against the daemon, shared by the TUI and the preflight |
| `internal/status` | both | The session status vocabulary and its sort order |
| `internal/install` | client | Merges the reporting hooks into Claude Code's settings |
| `internal/presence` | client | Session→process mapping, read back through `/proc` |
| `internal/compaction` | client | Compaction markers dropped by the `PreCompact` hook |
| `internal/localwatch` | client | The local mark saying a watcher is running here |
| `internal/reachability` | client | The local mark saying the daemon did not answer, so a hook stops waiting on it |
| `internal/usage` | client | Subscription usage fetch, held under a fleet-wide lease |
| `internal/animation` | — | Renders the README asset; not built into either binary |
| `internal/clock` | both | Time source, so tests do not wait |

### Boundaries

- The client (`internal/client`, `report`, `tui`) MUST NOT import
  `internal/server`, `internal/store`, or `internal/web` — that is what keeps the
  client binary minimal (enforced by `depguard` in `.golangci.yml`)
- `config`, `api`, and `version` are shared leaves — no business logic
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
| `time.Now()` | an injected `func() time.Time` (default `clock.Now`) |
| `os.Getenv()` | `internal/config` (the env-reading layer) |
| `http.DefaultClient` | a client with a timeout (`http.DefaultClient` has none) — a package-level one, **not** injected; see below |

Enforced by `forbidigo` in `.golangci.yml`. The seams are excepted: `internal/clock`
defines the wall clock, `internal/config` and `internal/daemon` (the composition
root) read env, and tests use the real ones.

**The HTTP client is the exception to "inject", deliberately.** The rule above
asks for injection and the first two rows get it; the third does not, and saying
so is the point of this paragraph — an intro promising more than its own table
delivers is how a rule stops being read.

The client is stateless, one configuration serves the whole binary, and the
packages that hold it (`apiclient`, `client`, `report`, `tui`, `watch`) expose
free functions rather than structs — `post(cfg, req)`, `getJSON(cfg, path, out)`.
Injecting would mean a constructor per package or a parameter on every signature
and every call site, for a dependency nobody varies in production. `var
httpClient` is a package-level **var**, so a test substitutes it, which is what
the seam is for.

What that costs, stated rather than hidden: a test that swaps the client mutates
process state, so it cannot run in parallel with another test in the same
package. Injection would fix that. It has not been worth the refactor; if it ever
is, this paragraph is what has to change with it.

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
