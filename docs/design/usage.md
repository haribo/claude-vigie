# Subscription usage — Design Specification

**Status:** Accepted.

Source of truth for the **usage** vigie reports — the Claude subscription
budget, not per-session token counts (those are in the session row). The
user-observable behavior, not the code.

---

## 1. What it shows

Claude's subscription enforces two rolling usage windows. vigie mirrors
both, as a strip under the sessions table:

- **5-hour window** — percentage of the short rolling limit used, with the time
  it resets.
- **7-day window** — percentage of the weekly limit used, with its reset time.

**Percentages only — never currency.** The fleet reports *how close to the
limit*, not a dollar figure. This is a deliberate privacy line: no cost or
billing amount is fetched, stored, or shown.

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

The server keeps only the last posted percentages and reset times; every
dashboard shows that same shared snapshot.

---

## Appendix — doc conventions (from tribnest)

`docs/design/` = the *what* (user-observable); `docs/adr/` = decisions with
rationale; code = the *how*. Docs never paraphrase code. Adding/modifying design
docs requires explicit consent — propose first.
