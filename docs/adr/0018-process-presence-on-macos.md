# ADR-0018: Read process presence through sysctl on macOS

## Status

Accepted. Amends [ADR-0006](0006-session-presence-via-proc.md), which stays in
force on Linux. An unofficial port: no macOS build is released.

## Context

ADR-0006 ties presence to `/proc`, and macOS has none. Running there anyway does
not fail loudly; it loses the signal every status derives from:

- **Capture no-ops.** The hook's ancestor walk cannot read a single process, so
  no session→process mapping is ever written, and a quiet session falls to the
  transcript heuristic's `ended`.
- **The registry carries no guard.** On macOS Claude Code writes `procStart` as a
  date — `"Mon Oct 12 09:56:18 2026"` — not as clock ticks. `ParseUint` read it as
  0, so `registryDead` never fired: a crashed session stayed `idle` on the board
  for good, and the `/clear` supersession check (#367) never matched.
- **Notifications** call `notify-send`, which macOS does not have.

**Measured** on Claude Code v2.1.296, macOS 14 (Darwin 23.6), three live sessions,
2026-10-12:

| question | answer |
|---|---|
| `kern.proc.pid` start time vs registry `procStart` | equal to the second, 3 of 3, registry in UTC |
| Claude Code's `p_comm` | `2.1.296` — the executable is its version file |
| Claude Code's `argv[0]` (`kern.procargs2`) | `claude` |
| `kern.proc.pid` for a pid with no process | succeeds with zero bytes; `x/sys` returns `EIO` |
| `kern.procargs2` for another user's process | `EINVAL` |

## Decision

**On macOS, read the process table through sysctl** (`golang.org/x/sys/unix`, no
cgo), behind the same three-valued `Status` as on Linux:

- **Identity.** Parent and start time from `kern.proc.pid`; the name from
  `argv[0]`'s base. `p_comm` is rejected: it names Claude Code's version, not
  Claude Code.
- **Start time** is seconds since the epoch, on both sides: the reader's
  `p_starttime`, and the registry's `procStart` parsed as `time.ANSIC` in UTC. The
  unit differs from Linux's clock ticks, but a mapping is only ever compared on the
  machine that wrote it.
- **Absent is `EIO`.** For a pid with no record the kernel returns no bytes and
  `x/sys` reports the size mismatch as `EIO`; that, `ESRCH` and a record for
  another pid read `Gone`. Everything else is `Unknown` — the #663 rule unchanged.
- **The watcher scan** (`WatcherRunning`) walks `kern.proc.all` and matches
  `argv[0]`'s base and a bare `watch` argument. Processes whose arguments the
  kernel withholds are not ours, and are skipped.

Around it:

- **Notifications** go through `osascript display notification`, title and body
  passed as `argv` and never spliced into the script. `terminal-notifier` was
  rejected: a third-party install for what the system already does. A graphical
  session is assumed unless the login is SSH.
- **The watcher** runs as a LaunchAgent ([deployment](../deployment.md)), and
  `vigie init` names it.
- **Config lives where it does on Linux**: `$XDG_CONFIG_HOME/vigie`, else
  `~/.config/vigie`. `os.UserConfigDir` was rejected: on macOS it is
  `~/Library/Application Support` and ignores `XDG_CONFIG_HOME`, so every path the
  docs give was false there — and every test that isolated itself through the
  variable read and wrote the operator's real config instead.
- **Any other platform** reads `Unknown` for every process — never `Gone`, which on
  a platform that cannot look would end every session.

## Consequences

- `idle` vs `ended` is as reliable on macOS as on Linux, including after a hard
  kill, and the registry path regains its pid-reuse guard.
- The Linux reader is untouched: `readStat` and its fake-`/proc` tests are the
  same code.
- **The tests that fake `/proc` test the Linux reader** and are built on Linux
  only; registry fixtures are written in the platform's `procStart` format. The
  macOS reader has its own tests, against the real kernel.
- **If Claude Code changes how it writes `procStart` or names its process on
  macOS, this is wrong again** — the guard is a measurement like the one above
  ([ADR-0016](0016-capture-claude-code-artefacts.md)), not a test, which only sees
  the sample we recorded. A single-digit day has not been observed yet; both C
  paddings parse.
- Unofficial: `.goreleaser.yaml` still builds Linux only, and the README says so.
