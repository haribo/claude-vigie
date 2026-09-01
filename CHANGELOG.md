# Changelog

All notable changes to claude-vigie are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project aims to
follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Each GitHub Release's notes are a mirror of that version's section below — this
file is the single source of truth, not a second narrative.

## [Unreleased]

### Fixed

- A subagent whose completion never arrives no longer pins its session to
  `working`. The next prompt you type closes it, as it already closed a tool call
  left hanging (#662).

## [0.10.0] - 2026-09-01

### Added

- The dashboard can call you. It notifies when a session starts calling — the rule
  the terminal and the GNOME indicator already follow — and `n` jumps to the one
  waiting longest. Off until you turn it on in Settings, and it needs an https or
  localhost address (#667).

## [0.9.1] - 2026-09-01

### Fixed

- The terminal notifies whenever a session starts calling for you, not only when
  it was working a moment earlier. A permission prompt arriving after a finished
  turn used to be audible on the desktop and silent in the terminal (#665).
- The dashboard's Stats periods say which window they are — `24h`, `7d`, `30d`,
  `1y`, `all`. `Week` meant the last seven days summed here and twelve stacked
  weeks in the terminal, with nothing saying which you were reading (#666).
- Stats says where its durations come from. A machine covered only by the
  watcher accrues tokens and never a second of time — only the reporting hooks
  close a status interval — and the empty panel used to read as an idle fleet
  (#668).
- A resumed session no longer reads `ended`. `claude --resume` keeps the session
  id, and the row came back announcing the end it had just left — with its old end
  time still attached — until the operator typed something (#664).
- A write error while rolling up tokens no longer costs a day its figure. The
  high-water mark could advance over a failed daily write, and `stats_daily` is
  never recomputed, so that output was gone for good (#669).
- The dashboard says when the board is not current. A failing refresh used to be
  discarded, so a frozen table sat under a green `live` chip, and a first load
  against an unreachable server drew a calm, empty fleet (#673).

## [0.9.0] - 2026-08-31

### Changed

- A session's colour now says how much attention it needs, and nothing else.
  Reasoning and compacting turn green like working, and `stale` takes `ended`'s
  grey in the browser with a hollow dot to tell them apart (#654).

### Fixed

- The session-retention setting now survives and governs. `off (keep all)` was
  read as "never set" and overwritten with 24 h at every daemon restart, deleting
  the sessions it was meant to keep; and a daemon started with
  `--session-retention=0` stored a window chosen in Settings without ever
  applying it (#656).
- A stuck `git` no longer costs a hook its report. Looking up the session's
  branch — done on every event, so once per tool call — was unbounded, and a
  held `index.lock` or a stalled mount spent the hook's whole budget on it,
  losing the status change and the heartbeat with it (#658).
- The `interrupted` marker now actually appears. A turn killed with Ctrl-C has
  been announced since 0.4.0, but the server blanked the marker on arrival, so an
  interrupted session had always looked exactly like one that finished (#659).
- `vigied token` prints the token and never invents one. Run against a daemon
  that takes its token from `VIGIE_TOKEN`, it used to find an empty database,
  generate a fresh secret, store it and print that — so the machine you then
  configured was refused (#657).

## [0.8.0] - 2026-08-30

### Changed

- The Sessions tab no longer carries its own warning lines. A watcher that stopped
  and a failed refresh are both named by the state pill and the modal behind `i`,
  which says which machine — the banner said `no watcher reporting` however many
  were, and stayed up for as long as the machine was off (#650).

### Fixed

- A machine that cannot read the account's usage no longer keeps the whole
  fleet's gauges empty. It took the fetch turn, failed — most plainly for want of
  local credentials — and kept renewing it, so no other machine ever fetched
  (#646).

- The browser and the terminal open the sessions table the same way. The browser
  put the session seen longest ago at the top, and the smallest token counts
  first, while the header arrow claimed the opposite (#645).

- Refreshing the vigie hooks no longer edits your own. It rewrote
  `settings.json` from a three-field idea of what a hook is, so a conditional
  hook came back unconditional and a prompt hook lost its model — and one of
  yours grouped with one of ours was deleted outright (#644).

## [0.7.2] - 2026-08-28

### Changed

- What a session is called, and how its permission mode and faults are spelled,
  are decided by the server rather than by each screen. A rule now reaches the
  terminal, the browser and the GNOME indicator at once (#618).

### Fixed

- The terminal and the browser show the same token count. A session at 1250
  tokens read `1.2k` in one and `1.3k` in the other, and round values lost their
  decimal in the browser only (#619).

- The GNOME indicator names a session the way the terminal and the browser do.
  One without a title showed as its project directory there and as its short id
  everywhere else, so the same session had two names (#618).

- The Stats tab's ranking names an untitled session by its short id, as the
  sessions table does, instead of printing the whole 36-character one (#630).

### Security

- Everything the terminal draws is checked before it is drawn, not sessions
  alone. A watcher build in the Machines tab, a session name in the Stats
  ranking, or a build named by the startup check could carry characters that act
  on the operator's terminal (#635).

- The daemon refuses a session report whose timestamp is not a real instant, and
  the terminal never prints one unchecked. A crafted report could act on the
  operator's terminal — set its window title — when they opened that session's
  detail panel (#629).

## [0.7.1] - 2026-08-26

### Changed

- Which sessions need you, and the order they sort in, are decided by the server
  rather than by each screen. The terminal, the browser and the GNOME indicator
  can no longer disagree about when to interrupt you — a new status reaches all
  three at once (#617).

- The context fill is computed by the server rather than by each client, so a new
  Claude model is taught to one place instead of two. The terminal and the browser
  now show the same percentage because it is the same number, not because two
  calculations agree (#616).

### Fixed

- The web dashboard says when a watcher has stopped reporting: the bottom bar
  raises the alarm and names the machines, and each machine card shows its own
  state. Until now the browser showed frozen statuses as if they were current,
  and only the terminal knew better (#623).

- The watcher warning covers every machine instead of the freshest one, and names
  the ones that stopped. A watcher dying on one host used to leave the indicator
  green for as long as any other machine kept reporting, while that host's
  sessions sat frozen on screen (#599).

- An unreadable watcher heartbeat now raises the alarm on both screens, instead
  of reading as healthy at the top of the TUI while the Machines tab called the
  watcher missing. Each says which fault it is, so it never sends you to the
  wrong machine (#600).

## [0.7.0] - 2026-08-20

### Added

- `vigie init` warns when the server URL is plain HTTP and its address is public:
  the token would travel in the clear on every request. It stays silent on a LAN
  or a private overlay, where plain HTTP is a legitimate choice, and it warns
  rather than refuses (#581).

### Changed

- The HTTP code of a live API error moves out of the STATUS cell into DETAIL, in
  the terminal and in the browser, and is spelled out: `● error` with
  `529 Overloaded` beside it. `error` was the only status carrying a refinement
  inside its own cell (#584).

- The hero images show the screen the TUI renders. They drew the summary strip,
  five rules and eleven key hints — a terminal from before #492, #493 and #494 —
  and a guard now compares their chrome against a live render (#571).

- The hero images are drawn from a template and rendered to both their copies,
  so they can be rebuilt and no longer drift apart — their two dark versions had
  ended up in different palettes. No pixel changed except the site's dark copy,
  which now matches the README's (#571).

- The usage strip spends its spacing inside the gauges rather than between them:
  the bar no longer runs into its percentage, and the two gauges are two spaces
  apart instead of four. One column narrower overall (#568).

### Fixed

- A session no longer waits on a daemon that has stopped answering. Every tool
  call used to pay a three-second timeout for as long as the outage lasted —
  minutes over a busy turn, with nothing said — and reporting resumes on its own
  once the daemon answers again (#578).

## [0.6.0] - 2026-08-17

### Added

- The web dashboard filters its session list on the TUI's rule: a
  case-insensitive subsequence match — `wapp` finds `web-app` — over name,
  machine, project, branch and status, plus the `rc` token. An untitled session
  is now named by the first eight characters of its id, as in the TUI (#545).
- The web dashboard groups its table by machine or project, with the same count
  and token subtotal per group as the TUI, and the operator's sort preserved
  inside each one. The mode is saved in the browser (#546).
- The web dashboard can hide sessions it has not heard from in a while, on the
  TUI's rule and steps. A session whose timestamp will not parse stays visible,
  and the `hidden` count now reports both reasons a row is missing rather than
  ended sessions alone (#547).
- The web dashboard shows the four columns it lacked: user, context fill, output
  tokens and permission mode. Context distinguishes *no reading* from *a reading
  of zero*, and an unrecognised permission mode is shown raw rather than
  relabelled. Saved column layouts survive the key renames (#550).

### Changed

- **Breaking:** the daemon refuses a usage snapshot from a machine that does not
  hold the usage lease, and rejects percentages outside 0–100. A watcher older
  than this is refused, so during a fleet upgrade the usage panel ages visibly
  instead of flapping (#515).
- **Breaking:** the daemon's token is supplied only through `VIGIE_TOKEN`;
  `--token` is removed and `FLEET_TOKEN` is no longer read, because a token on the
  command line is world-readable through `/proc/PID/cmdline`. Supplying nothing
  still generates, stores and prints one ([deployment](docs/deployment.md), #465).
- **Breaking:** session retention under an hour is refused — below that it deletes
  running sessions rather than old ones. It guards a mistake, not an attacker: Go
  durations have no month unit. The smallest window the TUI offers is 24 h, so no
  deliberate value is affected (#558).
- The Sessions tab spends 7 rows on chrome instead of 10, and 3 rules instead of
  5. The summary row is deleted — its counts were the exact aggregate of the
  STATUS column. `hidden N` moves to the bottom bar and shows only when something
  is hidden ([sessions-chrome](docs/design/sessions-chrome.md), #492).
- The eleven key hints are behind `h`, leaving one `h  help` on screen; they
  needed 134 columns and were truncated below that. `h` is the help key on every
  tab, which costs the two undocumented vim aliases in Settings. The `a` binding
  is removed (#493).
- Six scattered health indicators become one state pill and one modal behind `i`.
  The pill's three levels sort by whether something on screen is **false**, not by
  severity, so a Claude outage stays amber. The modal shows the whole observation
  chain in dependency order (#494).
- A degraded state pill breathes — amber and red animate, green never does —
  because a still character in a corner is not read. The cycle is two seconds,
  four times slower than the call marker, and no preference mutes it (#495).
- The web dashboard's contract with the TUI is written down: **"mirror" binds
  content and hierarchy, not gestures.** A divergence in what is shown is debt; a
  difference in how it is operated is not. Where the boundary is checkable it is a
  test rather than a sentence (#544).
- The web dashboard no longer draws the summary strip the TUI deleted in #492 —
  status counts, output total, `rc` count, aggregate sparkline. `hidden N`
  survives and moves to the bottom bar; showing ended sessions becomes a Settings
  preference (#548).
- Four guards now check what they claim: `depguard` covered eight of the client
  binary's sixteen packages and is derived from the dependency graph now, the auth
  test covered nine of eleven routes and reads them from the mux, the command
  guard covers `vigied` too, and a viewport assertion had been matching nothing
  since #492 (#556).
- `deployment.md` says what the shared token actually reaches: a holder can empty
  the fleet database and forge any session's status. It also says who holds it —
  the hooks read it, and they run inside Claude Code sessions. Splitting the
  credential was considered and rejected there (#558).

### Fixed

- The web dashboard keeps observing once its event stream is live. `ended` and
  `stale` are derived from the clock at read time, so they publish no event and
  reached a listener-only client never — a dead watcher stayed `working` under a
  green chip. It now polls and carries a silence watchdog (#538).
- The web dashboard marks every session that calls for the operator. It decided
  for itself and dropped `error`, so a session stuck on a 529 was drawn like any
  working one, and the tab badge counted `waiting` alone (#538).
- The session-status specification describes the machine that exists. It said
  twice that the watcher cannot observe `waiting`, which stopped being true when
  Claude Code began publishing a session registry, and it claimed a silent session
  simply reads `ended` where its own § 1 says `stale` (#539).
- The session id is stripped of control sequences like every other field the
  terminal renders. #529 cleaned twelve and left out the one nobody thinks of as
  text; its fixtures all used `ID: "a"`, which is why it went unseen (#540).
- Four guards check what they claim, and five documents describe the machine that
  exists: the watcher mark's cadence, the seven paths the client writes rather
  than two, its six commands rather than three, the GNOME description, and a
  pointer to a column-picker specification that does not exist (#557).
- ADR-0008 no longer states a consequence that #508 reversed: `compacting` does
  not clear a hook-posted `waiting`. The decision itself stands; only the
  overtaken sentence carries an amendment (#559).
- Three small defects: the desktop notifier left an unreaped child per
  notification, `Save` only tightened `config.toml` when it created it, and
  neither HTTP listener bounded idle connections (#560).
- The last specifications describing a deleted screen are corrected, and the
  architecture map lists the GNOME indicator, which had never appeared in it
  despite being a shipped client with its own schema and release path (#531).
- The terminal dashboard no longer relays control sequences. Session titles and
  detail text are transcript-derived, so a session could clear the operator's
  screen or write their clipboard at every refresh; the web escaped its side
  already (#529).
- The `vigie_output_tokens_total` metric can no longer grow the daemon without
  limit. Its `model` label came from the report body, and Prometheus keeps a
  counter per distinct value for the process's lifetime (#528).
- A malformed watch report can no longer freeze a session. A report carrying no
  status was taken for a hook and believed, inventing `working` and locking it
  there; a watch report without one is refused (#527).
- The daemon's database is readable by its owner alone. It holds the fleet token,
  and SQLite created it with the process umask — world-readable on a default one.
  Existing databases are tightened too, not only new ones (#526).
- The daemon validates what a report carries instead of trusting it. An unknown
  event fell through to `working` and was stamped hook-owned, which the watcher
  could then never retract — the #201 failure mode, reachable by any token holder
  (#515).
- The dashboard redraws when a session's effort, context or permission mode
  changes. The signature deciding whether a report is worth an SSE event omitted
  four fields the dashboard renders (#514).
- A hook installed from a path containing a space now reports. The command written
  into `settings.json` was built by concatenation and never quoted, so the shell
  split it (#513).
- A report can no longer overwrite another one's. The server read, merged in Go
  and wrote back with nothing holding the row in between; the cycle is atomic now
  (#512).
- The specifications describe the screen that exists: `session-list.md` was
  Accepted while still specifying the `a` binding and the summary strip, both
  removed weeks earlier (#511).
- `vigie init` no longer claims to install hooks. The watcher has owned them since
  ADR-0009, but `vigie help` and the README still described the old contract — and
  the README contradicted itself fifty lines apart (#510).
- The Machines tab no longer hides five statuses out of nine. Its per-machine
  counts came from a hand-written switch covering four (#509).
- A session blocked on a permission prompt no longer reads `stalled`. The prompt
  freezes the transcript on an unanswered tool call, which is exactly what a hung
  tool looks like, so a hook `waiting` is held until the transcript moves (#508).
- The README's animation shows the Sessions tab the TUI actually renders — it
  still drew the status-count row, the old gauge spacing and no state pill (#505).
- A session that calls you always blinks. The `blink` preference is removed rather
  than exposed: it was the one setting whose "off" state was invisible (#490).
- The key-hint footer costs one row at any width. It was wrapped rather than
  fitted, and the Sessions hints need 134 columns (#487).
- The summary strip keeps the view state on a narrow terminal. Its two halves did
  not share a width budget, so the left block was fitted against the full width
  and the right one had no room left (#486).
- A tool call that never got its result no longer pins a session to `stalled` for
  the rest of its life. A real operator prompt now closes every older unresolved
  call (#483).
- A preferences file that cannot be read is kept, not replaced. `loadPrefs` fell
  back to the defaults, and the next keystroke wrote them over the file (#480).
- Running the tests no longer overwrites the operator's own TUI preferences — a
  test sent `g`, which saves the view state into the real
  `~/.config/vigie/tui.toml` (#479).
- The deployment guide documented Prometheus metrics that do not exist: it still
  named them `fleet_*`, where the server has emitted `vigie_*` since the rename
  (#478).
- The old brand is gone from the identifiers that ship to users — the GNOME
  indicator's classes, the dashboard's `cf_token` and `cf_columns` storage keys,
  and three package comments. The storage keys are carried over, not dropped
  (#478).
- `TestWatcherRunning` no longer fails at random on CI. Its probe forked a shell,
  and between the fork and the `execve` the child carries the parent's name
  (#476).
- The GNOME indicator raises a badge and a notification for every signal that
  calls for the operator, not only `waiting`. A stalled turn and a session's own
  call raised nothing (#466).
- Sorting by status now places every status, in both dashboards. Only five of the
  nine were ranked, so a session hitting an API error sorted below `ended` (#464).
- The event stream notices its own death in seconds rather than minutes. A
  suspended machine's connection dies with no FIN or RST, so the read blocked on a
  socket that would never deliver another byte (#457).
- A failed poll no longer blanks the sessions table. Resuming from suspend showed
  an error and no sessions at all, for the minutes the connection took to return
  (#456).
- A panel now says when it is showing figures it could not refresh. The TUI
  fetches seven things from the daemon; only a sessions failure ever reached the
  operator (#449).
- An abandoned metadata file is no longer reported as a session. Claude Code
  writes a sidecar next to each conversation, and a renamed or moved project left
  one behind (#448).
- A synthetic assistant line no longer becomes the session's model. Claude Code
  writes `"model":"<synthetic>"` on lines it generates itself, and nothing
  filtered it (#433).
- Daily token stats could be inflated by orders of magnitude, permanently. The
  rollup counted the growth of the session's own counter, so any regression added
  a whole lifetime of output; it counts against a mark it owns now (#432).
- The nine session statuses are declared once and checked everywhere. The list was
  hand-copied into four consumers, each incomplete in a different way, which is
  how the two defects below shipped (#423).
- The GNOME extension no longer hides sessions. Its menu iterated a hand-written
  list of four statuses, so five others matched no group and were dropped (#422).
- The `vigie_sessions` gauge counts `compacting` sessions. It tallied every status
  but emitted a series only for those in its own list (#421).
- The reporting hook no longer re-reads the whole transcript at the end of every
  turn on a watched machine. `Stop` parsed from byte 0 inside a hook with a 5 s
  timeout, and the largest transcript measured took 11 s (#420).

## [0.5.0] - 2026-08-13

### Added

- A session can raise an explicit **call** for the operator: ask in plain
  language and Claude runs `vigie call "backfill done — 12k rows"`. The session
  raises it and the session clears it, so no action on vigie is ever needed
  ([ADR-0010](docs/adr/0010-session-raised-operator-call.md), #388).
- The TUI surfaces a call by motion: the status dot blinks at 1 Hz in its own
  colour and the message takes the `DETAIL` cell. A call jumps ahead of inferred
  attention states in the `n` queue. `blink` and `call_marker` in `tui.toml`
  adjust it (#389).
- The web dashboard surfaces a call with the same grammar: the pill's dot pulses,
  the row takes the attention border, the message fills Detail. No new colour, and
  `prefers-reduced-motion` stops the pulse (#390).
- `vigie` installs a personal Agent Skill so Claude knows the `vigie call` command
  exists without per-project setup. The watcher refreshes it at startup. The call
  is best-effort: if Claude does not run it, nothing is raised
  ([design](docs/design/call-discoverability.md), #391).
- The sessions table scrolls within a vertical viewport instead of spilling off
  the bottom of the terminal. Header and footer stay pinned; a `rows a–b / n`
  indicator appears only when the list overflows
  ([design](docs/design/tui-viewport.md), #378).

### Changed

- The `DOING` column is now `DETAIL`, and `GET /api/sessions` returns `detail`
  instead of `activity`. Most of what the column carries is not an action, and
  `ACT` already meant something else. A saved column layout is migrated (#393).
- A watcher whose build does not match the daemon can no longer write session
  state: the report is refused with `409`, while the machine and its build stay
  visible in the Machines tab. The watcher goes inert and resumes on its own
  ([design](docs/design/version-consistency.md), #384).
- **Breaking:** `vigie init` writes the config and nothing else — the watcher now
  owns the hooks and the call skill and keeps them matching its own build.
  `vigie init --uninstall` is removed; use `vigie hooks uninstall`, which also
  removes the skill ([ADR-0009](docs/adr/0009-watcher-managed-hooks.md), #415).
- `vigie init` asks for the server URL, the token and the machine name, and takes
  no flags. The token is read without echo, so the shared secret stays out of
  shell history (#407, #415).

### Fixed

- A machine whose watcher runs but has no session to report no longer reads as
  having no watcher. Liveness was a side effect of session data; the watcher now
  claims it every 5 s over its own endpoint
  ([design](docs/design/watcher-liveness.md), #386).
- Desktop notifications could be impossible with nothing saying so — a terminal
  that never reports focus events suppressed every one. Not knowing no longer
  counts as "you are watching", and Settings says why delivery fails (#411).

## [0.4.1] - 2026-08-08

### Fixed

- The daemon no longer returns intermittent `500`s on reports under load.
  `busy_timeout` was applied to the first pooled connection only, so every other
  one failed a contended write immediately; the pragmas now travel in the DSN
  (#372).
- The TUI startup preflight no longer reports a running watcher as down. A stale
  server heartbeat is a failed round-trip, not proof of death, so it cross-checks
  a local `/proc` signal before blaming the watcher (#371).

## [0.4.0] - 2026-08-07

### Changed

- `vigie watch` now owns the reporting-hooks lifecycle: it re-installs its own
  leg into `~/.claude/settings.json` at startup, so the installed hooks always
  match the running watcher — a service restart after an upgrade self-heals stale
  hooks (e.g. a missing `PreCompact` or a moved binary). The settings write is now
  atomic (temp + rename); a refresh failure is logged and never stops the watch
  ([ADR-0009](docs/adr/0009-watcher-managed-hooks.md)). Manual `vigie init` /
  `vigie hooks` still work.

### Fixed

- `vigie hooks` no longer advertises the removed `--detailed` flag in its usage,
  and an unknown flag now prints the real usage instead of a bare "Usage of hooks:".

### Added

- `vigie tui` now runs a startup preflight before entering the alt-screen: it
  requires a reachable server, a valid token, and a daemon whose build strictly
  matches the client's (commit-compared when either side is a `dev` build). Any
  failure prints both versions and the remediation and exits 1 — no more silent
  degradation behind a full-screen UI, and no bypass flag
  ([design](docs/design/tui-preflight.md)). When the local machine has vigie hooks
  installed, the preflight also requires a fresh, version-matching local watcher —
  a hooks-only machine with a dead or outdated watcher reports stale statuses.
- The Machines tab now shows each machine's watcher version. The watcher reports
  its build in the heartbeat, the server stores it per machine and returns it from
  `GET /api/watcher`, and both the TUI (a VERSION column) and the web dashboard
  (a per-machine chip) display it — so a watcher that has drifted behind the
  daemon is visible.
- Sessions the operator interrupted (Ctrl-C/Esc) now show an `interrupted` marker
  in the activity column instead of a bare `idle`, so a turn killed mid-flight is
  distinguishable from one that finished cleanly. It clears with no timer — the
  next real prompt or reply replaces it. A DOING refinement, not a new status.

## [0.3.0] - 2026-08-07

### Added

- New `compacting` status: while a session summarizes its context (a silent
  ~90–170 s the registry reports as `busy`), vigie now shows `compacting` instead
  of an opaque `working`, so the context-gauge drop becomes legible. Detected via
  a new `PreCompact` hook (start) and the transcript's `compact_boundary` (end),
  with a safety timeout; it is a display refinement of `working`, not an
  attention status ([ADR-0008](docs/adr/0008-compacting-status.md)).
- The `vigie` client and `vigied` daemon build versions are now visible in the
  dashboards: a Build section in the TUI Settings tab shows both and flags a
  client/daemon drift, and a `vigied <version>` chip sits in the web topbar
  (commit and build time in its tooltip). Served over a new `GET /api/version`.

### Fixed

- A session whose only work runs inside async subagents (the `Task`/`Agent` tool)
  now reads `working`, not `idle`. Vigie tracks in-flight subagents from the
  parent transcript alone — opening on the launch, closing on its
  `task-notification` — with a liveness cap that self-heals a missed close, and
  shows `N agents: <description>` in the activity column.

## [0.2.0] - 2026-08-05

### Changed

- TUI: tightened the fixed-width columns (`TOTAL`, `EFFORT`, `OUT`, `CTX`, `RC`)
  to their real content, reclaiming ~8 columns of width so more fit before the
  table overflows on a narrow terminal. No content is truncated.

### Fixed

- TUI summary strip now drops its trailing elements whole — activity first, then
  rc, then out — when the terminal is too narrow, instead of clamping and cutting
  the activity sparkline mid-glyph. The status counts are always kept.
- TUI bottom usage/platform strip no longer overflows a narrow terminal: it now
  drops the secondary platform indicator when the two don't fit and clamps the
  usage side as a last resort. The width-sweep scaling guard's fixture is now
  fully populated (usage, platform, activity history), so it actually exercises —
  and would catch — this class of overflow.
- TUI Machines and Settings tabs no longer overflow the terminal width on a
  narrow terminal: the machines overview table clamps each row to width, its
  no-watcher help text wraps, and the column-picker header wraps. The width-sweep
  scaling guard now covers all three interactive tabs.
- TUI Sessions view no longer overflows the terminal width on resize: the
  summary strip keeps its status counts and drops the secondary sort/connection
  side when space is tight, the key-hint footer wraps, and the column auto-drop
  now accounts for the row's left gutter so the table never spills over by a
  column or two.
- TUI: the "N columns hidden" overflow banner now wraps to the terminal width
  instead of running past the edge and being cut off on a narrow terminal — the
  message that explains the narrowness is no longer itself unreadable.
- Column picker: hiding a column now keeps its position instead of jumping it to
  the bottom, and reordering works on any column — hidden or visible (TUI + web).
- TUI column picker is now width-aware: it shows the width budget, flags every
  selected column the terminal is too narrow to fit, and a banner names the
  columns dropped instead of hiding them silently (the TUI never scrolls
  sideways).
- TUI `a` key now toggles the persistent "hide ended" setting directly (saved,
  shared with Settings) instead of a separate transient override that could
  diverge from it and appear broken.

## [0.1.0] - 2026-08-04

First release. claude-vigie is an observe-only monitor for Claude Code sessions
across machines — it reads and reports session state; it never drives a session
([ADR-0005](docs/adr/0005-observe-only.md)) and holds no operator-handling state
([ADR-0007](docs/adr/0007-read-only-to-operator.md)).

### Added

- **Architecture** — two static, CGO-free Go binaries: the daemon `vigied` (HTTP
  + SSE API, embedded SQLite) and the client `vigie` (hooks reporter, transcript
  watcher, terminal dashboard). One server, a client on every machine.
- **Session model** — seven live statuses (working, thinking, waiting, stalled,
  idle, error, ended) plus `stale` for a session on a machine with no watcher.
  Status and presence come from Claude Code's own session registry and
  transcripts; two observers (hooks + watcher) reconciled by authority.
- **Terminal dashboard (TUI)** — sessions table with per-session status, tokens,
  activity, and a detail view; single-glyph status indicators; sort, filter,
  group, and column select/reorder — all persisted.
- **Web dashboard** — a read-only browser mirror of the TUI, served by the
  daemon, with the same status/sort/column controls (client-local).
- **Attention** — desktop notifications when a session starts waiting, a
  next-waiting hotkey, machines-tab flagging of hosts running on hooks alone, and
  a GNOME Shell top-bar indicator.
- **Per-session insight** — reasoning effort, context-window usage percentage,
  permission mode (manual/accept/plan/auto/bypass), and the remote-control resume
  URL, all detected read-only.
- **Operations** — Prometheus metrics on a dedicated, unauthenticated ops
  listener (never the API port), a portable Grafana dashboard, and Claude platform
  status polled from status.claude.com.
- **Landing site** — the static claudevigie.org one-pager.

### Security

- The API binds `127.0.0.1` by default; every `/api/*` route is behind a
  constant-time shared-token check; request bodies are size-capped.

[Unreleased]: https://github.com/haribo/claude-vigie/compare/v0.10.0...HEAD
[0.10.0]: https://github.com/haribo/claude-vigie/compare/v0.9.1...v0.10.0
[0.9.1]: https://github.com/haribo/claude-vigie/compare/v0.9.0...v0.9.1
[0.9.0]: https://github.com/haribo/claude-vigie/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/haribo/claude-vigie/compare/v0.7.2...v0.8.0
[0.7.2]: https://github.com/haribo/claude-vigie/compare/v0.7.1...v0.7.2
[0.7.1]: https://github.com/haribo/claude-vigie/compare/v0.7.0...v0.7.1
[0.7.0]: https://github.com/haribo/claude-vigie/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/haribo/claude-vigie/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/haribo/claude-vigie/compare/v0.4.1...v0.5.0
[0.4.1]: https://github.com/haribo/claude-vigie/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/haribo/claude-vigie/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/haribo/claude-vigie/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/haribo/claude-vigie/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/haribo/claude-vigie/releases/tag/v0.1.0
