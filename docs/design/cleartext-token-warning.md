# Cleartext token warning — Design Specification

**Status:** Accepted (#581).

Source of truth for **when `vigie init` tells the operator that the fleet token
would travel in the clear**. The boundary is the whole subject: the rule is one
line, and every decision in it is about where that line sits.

---

## 1. Why the warning exists

The token authenticates every request, and `PostToolUse` is installed by default,
so it rides several requests per turn. `deployment.md` states what that costs on
an untrusted network:

> **The shared token is only meaningful over TLS on a public network.** It travels
> in the `Authorization` header on every request; without TLS it is captured on
> the wire, and whoever captures it gets full read/write on the API.

That is written in a deployment guide. `vigie init` is the path taken by someone
who has not read the deployment guide — it is where the address is chosen — and
it said nothing.

## 2. Decision

**`vigie init` warns when the scheme is `http` and the server's address is
public.** It warns; it never refuses.

An address is *not* public when it is loopback, RFC 1918 (`net.IP.IsPrivate`,
which also covers the IPv6 ULA range `fc00::/7` a WireGuard mesh usually hands
out), link-local, unspecified, or inside **`100.64.0.0/10`** — the RFC 6598
shared address space, which is what Tailscale assigns and which `IsPrivate` does
**not** cover.

A name is resolved to decide. A resolution that fails yields no warning: one that
cannot be justified is noise, and `init` already refuses a server it cannot
reach. A name answering with both a private and a public address warns — the
token would take the public path.

## 3. Rejected: warn whenever the host is not loopback

This was the rule the issue specified. It was implemented first and run against
the case table, which is the only reason its cost is known rather than argued:
**it fires on six legitimate configurations**, counted, not estimated.

| case | `deployment.md`'s position |
|---|---|
| `192.168.1.10` | "Plain HTTP is fine on a trusted network" |
| `10.0.0.5` | same |
| `172.20.0.9` | same |
| a LAN name resolving to a private address | same |
| `100.101.102.103` (Tailscale) | a private overlay is what it **recommends** |
| an IPv6 ULA (WireGuard) | same |

A seventh case fails too and is not a configuration: a name that does not
resolve. The rejected rule warned about a host it had learned nothing about,
which is § 2's reason for staying silent there.

The Tailscale and ULA rows are the ones that settle it. `deployment.md` § *Recommended for
dynamic client IPs* says a private overlay "is the simplest robust option" and
that `vigied` then "needs **no public port at all** — zero internet attack
surface, no certificate to manage". A warning that fires on the recommended
configuration is ignored, and it teaches the operator to ignore the next one. It
would make the tool less safe, not more.

## 4. A warning, never a refusal

Plain HTTP on a trusted network is a legitimate deployment — `deployment.md` says
so — and which networks are trusted is the operator's to know and not the
client's to decide. Refusing would also be useless: the operator would work around
it, and the workaround would be silent where the warning is not.

This is the same line as [ADR-0007](../adr/0007-read-only-to-operator.md): vigie
reports what it sees and leaves the operator in charge of it.

## 5. Why `init`, and only `init`

The warning belongs where the address is chosen, once, and where a human is
present to read it.

**Not `config.Load`**, which is the tempting place because it is the one every
path shares. It runs inside every reporting hook, so it runs on every tool call —
printing there would put a security warning in the middle of the operator's work
several times a minute, which is how a warning becomes noise. It is also the path
[unreachable-daemon](unreachable-daemon.md) exists to keep cheap.

**Printed last**, after `init`'s success block, so it is what the operator is left
looking at rather than something scrolled past on the way to "configured".

## 6. Consequences

- A hand-edited config is not covered. Nothing warns, because nothing chose an
  address interactively. A once-at-startup warning in the watcher would close
  that gap and is not specified here.
- The rule depends on name resolution, so a server whose name resolves
  differently from the machine that ran `init` is judged on what `init` saw.
- The boundary follows `deployment.md`. If that document ever stops calling
  trusted-network HTTP fine, or stops recommending an overlay, this rule is what
  has to move with it — they are one decision written in two places, and § 3 is
  the reason.
