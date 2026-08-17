# Changelog conventions

See also: [git-workflow.md](git-workflow.md) for the release flow,
[git-commits.md](git-commits.md) for commit messages.

`CHANGELOG.md` follows [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/)
and [Semantic Versioning](https://semver.org/). It is the **single source of
truth** for release notes: a GitHub Release body is a mirror of its version's
section, never a second narrative.

## The entry

**One or two sentences.** What changed, from the reader's side, plus the issue
number. That is the whole entry.

```
- The web dashboard filters its session list, on the TUI's rule — a
  case-insensitive subsequence match plus the `rc` token (#545).
```

Keep a Changelog states the test this rule comes from:

> The changelog entry should be **the headline and the hook, not the full story**.

The full story already exists and is already linked. The issue holds the problem,
the PR holds the reasoning and the trade-offs, the code holds the *why* in its
comments. An entry that re-tells any of that is a fourth copy, and the one nobody
maintains.

**Write for someone deciding whether to upgrade**, not for someone auditing the
change. They want to know what will be different on their screen and whether
anything will break. They do not want the mechanism.

## Rules

1. **One or two sentences**, hard ceiling 60 words. A guard enforces it on
   `[Unreleased]` — see below.
2. **Reference the issue**: `(#N)` at the end. It is what makes brevity safe.
3. **Say what changed for the reader**, not how it was implemented. No file
   paths, no function names, no test names.
4. **Breaking changes open with `**Breaking:**`** and say what an operator has to
   do differently.
5. **One entry per change**, in the category it belongs to. Not one per commit —
   a changelog is not a commit log.
6. **Categories in Keep a Changelog's order**: `Added`, `Changed`, `Deprecated`,
   `Removed`, `Fixed`, `Security`. Only the ones a version needs.
7. **Deprecate before removing.** A feature that disappears without a prior
   version announcing it deprecated leaves no upgrade path. `--token` and
   `FLEET_TOKEN` were removed in 0.6.0 with no such notice; that is the mistake
   this rule exists to stop repeating.

## Why the ceiling is enforced, and only on `[Unreleased]`

`git-workflow.md` has always said "adds **a line**". It drifted anyway: by 0.6.0
the median entry was 138 words and the longest 218 — paragraphs explaining
mechanisms, rejected alternatives and failure modes, all of which belonged in the
PR that carried them.

A rule nobody can fail is a rule that erodes, so
`test/docs/changelog_test.go` counts the words. 60 words is loose — two full
sentences with a reference fit comfortably — so the guard fires on essays, never
on prose.

It guards `[Unreleased]` only, because that is where entries are written and
where the rule is still free to apply. The released sections were brought into
line **once**, deliberately, when this rule was adopted: 0.4.1, 0.5.0 and 0.6.0
were rewritten, every entry and every issue reference kept, and their GitHub
Release bodies republished to stay mirrors. 0.1.0 to 0.4.0 were left alone — they
were already short, and they carry no issue references, so their text is the only
record there is. Compressing those would have destroyed information to fix a
problem they did not have. That is the trade to weigh if it ever comes up again.

## Before and after

From 0.6.0, an entry as it was written and as it should have been:

> **Before** (140 words). *"**Breaking:** the daemon refuses a session-retention
> window shorter than an hour. Below that the setting stops being a retention
> policy and becomes a delete button: the prune loop takes `now - retention` as
> its cutoff and removes every session, event and token sample older than it —
> running sessions included, since the predicate is last-report time and not
> status. This is not a security control, and it is not sold as one: anyone who
> can set it holds the fleet token…"*

> **After** (28 words). *"**Breaking:** session retention under an hour is
> refused — below that it deletes running sessions rather than old ones. No
> deliberate value is affected; the smallest the TUI offers is 24 h (#558)."*

Everything cut is in #558 and its PR, where a reader who wants it will look, and
where it is maintained.
