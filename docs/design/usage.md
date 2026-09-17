# Subscription usage — Design Specification

**Status:** Accepted.

Source of truth for the **usage** vigie reports — the Claude subscription
budget, not per-session token counts (those are in the session row). The
user-observable behavior, not the code.

---

## 1. What it shows

Claude's subscription enforces rolling usage windows. vigie mirrors them as a
strip under the sessions table:

- **5-hour window** — percentage of the short rolling limit used, with the time
  it resets.
- **7-day window** — percentage of the weekly limit used, with its reset time.
- **A weekly limit scoped to one model**, when one is in force — the model's name,
  its percentage, and the reset. Drawn beside the 7-day gauge behind a middle dot,
  because it is the same window (#840).

**The scoped gauge is read from the endpoint's own list of what it enforces**, not
from a field named after a model. Each entry declares what it applies to, and the
one vigie draws is the weekly entry that names a model. The payload still carries
flat per-model keys and they are empty: that shape was abandoned upstream, and
reviving it here would mean a list of model names to keep in step by hand — the
next model to carry a limit would be invisible in turn.

**Its label is the name the endpoint gives**, lower-cased in the terminal and
upper-cased by the browser's stylesheet, like `5h` and `7d`. vigie does not
translate it: a name invented here would be a second vocabulary to hold against
Claude's.

**Nothing is drawn when no limit is scoped, and no room is reserved.** A gauge
permanently at zero trains the eye to skip the place where the exception appears —
the rule the `hidden N` marker already follows.

**The reset of the weekly pair is written once, and only while the two instants
match.** They are one window, so one reset is the honest rendering; if the endpoint
ever reports two, both are printed, because dropping one would then say something
false about the other. It also keeps the terminal line inside a narrow terminal,
which matters — the strip is clamped and the TUI never scrolls sideways.

**Severity is deliberately not shown.** The endpoint grades each limit
`normal`/`critical`; the percentage already says it, and a second encoding of the
same fact is a row the eye learns to skip. The same goes for the flag marking which
limit is currently binding: with every gauge on screen, the highest is the binding
one.

**Percentages only — never currency.** The fleet reports *how close to the
limit*, not a dollar figure. This is a deliberate privacy line: no cost or
billing amount is fetched, stored, or shown. The endpoint returns spending figures
in the operator's own currency, and vigie does not parse them.

---

## 2. One fetcher for the whole fleet

Usage is an account-wide fact, identical on every machine — so exactly one
machine fetches it, and the rest read the shared result:

- **Lease.** A machine must hold a short usage lease before fetching; only the
  lease holder fetches. This keeps N machines from hammering the endpoint in
  parallel and gives the fleet a single, consistent figure.
- **Local credentials, token stays put.** The holder reads the account's usage
  from the local OAuth credentials on that machine. **The token never leaves the
  machine** — only the resulting percentages and reset times are posted to the
  server.
- **Backoff.** The usage endpoint is aggressively rate-limited, so the fetcher
  backs off exponentially on failure (a circuit breaker) rather than retrying
  tightly.
- **A holder that cannot fetch gives the lease back.** The lease is a right to
  fetch, not a right to hold: a machine that takes it and then fails — most
  plainly because it has no local credentials — hands it back so the next machine
  can try. Without that, one machine with nothing to read empties the gauges for
  the whole fleet, permanently, and an empty gauge reads exactly like one nobody
  has filled yet.

  A holder that *crashes* needs no rule: it stops renewing and the lease lapses.
  What needs one is the holder that keeps asking and never delivers.

The server keeps only the last posted percentages and reset times; every
dashboard shows that same shared snapshot.

---

## Appendix — doc conventions

`docs/design/` = the *what* (user-observable); `docs/adr/` = decisions with
rationale; code = the *how*. Docs never paraphrase code. Adding/modifying design
docs requires explicit consent — propose first.
