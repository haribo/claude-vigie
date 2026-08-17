# Changelog

All notable changes to claude-vigie are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project aims to
follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Each GitHub Release's notes are a mirror of that version's section below — this
file is the single source of truth, not a second narrative.

## [Unreleased]

### Added

- The web dashboard shows the four columns it was missing: the session's user, how
  full its context is, its output tokens on their own, and its permission mode.
  All four already travelled in the API; the browser simply never drew them, so
  `plan` against `bypassPermissions` — which decides what a session may do without
  asking — was legible in the terminal and invisible in the tab left open all day.
  Context distinguishes *no reading* from *a reading of zero*, as the daemon does
  and for the reason it does (#367): a dash and `0%` are different facts. An
  unrecognised permission mode is surfaced raw rather than relabelled, so a mode
  added tomorrow cannot read as the safe default (#304). Both rules are checked
  against the Go originals through a shared fixture rather than trusted.
  Four column keys are renamed to the TUI's — `project`, `tokens`, `activity` and
  the detail button's `act` — and a saved layout is carried across the rename
  once, under a new storage key, so no operator loses their order or has a hidden
  column reappear. A Go test now compares the two column sets and names what is
  missing, which is what makes the next divergence a CI failure instead of an
  audit finding (#550).
- The web dashboard can hide sessions it has not heard from in a while, on the
  TUI's rule and with the TUI's steps. Three details of that rule are load-bearing
  and are now shared rather than reinvented: the clock is `last_seen_at` and not
  the status, so a `working` session whose reports stopped is hidden too — the
  same "nothing is happening here" signal; a timestamp that will not parse keeps
  its session **visible**, because losing a row over a bad date is worse than
  showing one too many; and off is off, and is the default. The `hidden` count now
  reports both reasons a row is missing rather than counting ended sessions alone,
  which is the whole justification for that count existing — without it the screen
  claims a fleet of one while three machines are reporting
  ([sessions-chrome](docs/design/sessions-chrome.md) § 2, #547).
- The web dashboard groups its table by machine or by project, as the TUI does.
  Groups come out in key order while the operator's sort survives inside each
  one — a stable re-sort on top of the active sort, not a single comparison that
  would flatten it — and each group carries the two figures the terminal shows:
  how many sessions, and their combined tokens across all four buckets rather
  than output alone. The project key is the last path segment, so the same project
  checked out under different roots on two machines lands in one group. Group
  headers are rows of the same table, not a table per group: separate tables would
  each pick their own column widths and the eye could no longer read down a
  column. The mode is stored in the browser with the column layout and the sort,
  and an unrecognised stored value degrades to ungrouped rather than blanking the
  table. A Go test reads the mode names out of the shipped `lib.js`, so a
  vocabulary renamed on one client and not the other cannot pass CI (#546).
- The web dashboard filters its session list. The TUI has had `/` since the
  beginning; the browser — the window an operator actually leaves open all day —
  made them scroll instead. Same rule as the terminal: a case-insensitive
  subsequence match, so `wapp` finds `web-app`, over the session's name, machine,
  project, branch and status, plus the `rc` token that selects the
  remote-controlled sessions rather than text-matching. Under the boundary written
  in #544 that rule is content, so it is shared rather than restated — the
  JavaScript port and the Go original are run against one fixture and must agree
  case for case, since a rule copied per consumer is exactly what #421, #422 and
  #466 were. The input sits outside the region the refresh tick repaints, so a
  redraw cannot take the caret or the typed text with it. An untitled session is
  now named by the first eight characters of its id, as in the TUI: the column
  showed all thirty-six while the filter could only reach eight (#545).

### Changed

- Four guards now check what they claim. `depguard` enforces ADR-0003's barrier
  over a list of packages kept by hand, and the list was eight short: `api`,
  `apiclient`, `clock`, `compaction`, `config`, `status`, `transcript` and
  `version` all ship in the `vigie` binary and sat outside it, so an import of
  the store added to any of them would have compiled, linked and passed CI. The
  list is no longer trusted — a test derives it from `go list -deps ./cmd/vigie`.
  The route test that `deployment.md` cites by name as the evidence the API is
  closed covered nine of eleven paths; it reads them out of the mux now, so it
  cannot be short again. The command guard read only the `vigie` lines of a block
  titled "Two binaries", which is how `vigied stats-repair` came to be documented
  in the architecture map, printed by the CLI, and missing from the README. And a
  viewport assertion labelled "summary counts missing" had been matching a STATUS
  cell since #492 deleted the summary — it would have passed with no header at
  all; it checks the pinned column header now, and the specification that still
  asked for the summary is amended (#556).

- The web dashboard no longer draws the summary strip the TUI deleted in #492 —
  the status counts, the output total, the `rc` count and the aggregate activity
  sparkline. The tempting defence, that a browser has room a 24-row terminal does
  not, is not the reason those went: sessions-chrome.md § 2 turns on *it is not
  already on screen*, and `● working 3` is the exact aggregate of the STATUS
  column whether the screen is 80 characters or 1920 pixels. `hidden N` survives
  on its own merit, moves to the bottom bar where the terminal keeps it, and is
  shown only when something is actually hidden — a permanent zero trains the eye
  to skip the place where the exception appears. With that count no longer
  permanent, the control that used to sit on it moves too: showing ended sessions
  is a Settings preference now, beside the idle window, as it is in the TUI. The
  per-session activity sparkline stays in its column, where it says something the
  table cannot (#548).

- **Breaking:** the daemon refuses a usage snapshot from a machine that does not
  hold the usage lease, and rejects percentages outside 0–100. The lease exists so
  exactly one machine fetches, and nothing checked it at the write, so any holder
  of the token could overwrite the figure the whole fleet reads, with any value.
  A watcher older than this change posts no holder and is refused — during a fleet
  upgrade the usage panel therefore ages instead of flapping, and the TUI's state
  modal reports that age, so the staleness is visible rather than silent (#515).
- **Breaking:** the daemon's shared token is supplied one way only, through
  `VIGIE_TOKEN`. The `--token` flag is removed and `FLEET_TOKEN` is no longer read.
  A token on the command line is published to every local user through
  `/proc/PID/cmdline`, which is world-readable, while `/proc/PID/environ` is
  readable only by the process owner — measured, not assumed. Two ways to pass a
  secret, where the more discoverable one leaks it and took priority, is a trap
  rather than a convenience. The order is now `$VIGIE_TOKEN` → the stored token →
  a generated one; the environment still wins over the store, so a token set
  explicitly beats one the daemon persisted for itself. The default path is
  unchanged and remains the safest: supply nothing, and the token is generated,
  stored and printed once, with `vigied token` to read it back
  ([deployment](docs/deployment.md), #465).
- A degraded state pill breathes. One character in a corner will not be read if
  it is still: peripheral vision detects motion and luminance, not hue or shape,
  and since no text ever appears beside the pill, the pulse *is* the alert. Amber
  and red both animate, green never does. The glyph is always present — only its
  color modulates, toward a second tone of the same hue, chosen by hand for each
  theme because the TUI cannot know the terminal's background. The cycle is two
  seconds, four times slower than the call marker's: the cadence is what separates
  "come now" from "still broken", and a slow one keeps a long-lived degraded state
  from pinning the TUI to a redraw loop. There is no preference to mute it, for
  the same reason `blink` was removed — an alert the operator cannot see is worse
  than no alert. 0.5 Hz sits well under WCAG 2.3.1's three-flashes-per-second
  ceiling ([sessions-chrome](docs/design/sessions-chrome.md), #495).
- Six scattered health indicators become one state pill and one modal behind `i`.
  The connection glyph, the watcher warning, the two "could not refresh" marks and
  the `⟳` freshness glyph all asked one question at five granularities — *is what
  I am looking at true?* — each with its own glyph, wording and place; the sixth,
  `platform ● operational`, was not about vigie at all and read the same 99 % of
  the time. The pill sits in the tab-line corner, preceded by its `[i]` keycap,
  and never changes width. Its three levels are sorted by whether something on
  screen is **false**, not by severity: a Claude outage stays amber, because vigie
  is correctly reporting sessions that are correctly erroring. `i` opens the whole
  observation chain, in dependency order, with the client/daemon version drift
  that until now was buried in Settings. When the server is unreachable, what the
  TUI establishes itself goes red, what it only knows *through* the server goes
  grey — unknown, not failed — and the usage snapshot stays amber with its age,
  which is the one thing still true about it offline. The failure banners are
  unchanged: the modal is added to them, not substituted for them
  ([sessions-chrome](docs/design/sessions-chrome.md), #494).
- The eleven key hints are behind `h`. They sat permanently at the bottom of the
  Sessions tab, needed 134 columns, and below that were dropped one by one with an
  ellipsis — a row both permanently occupied and incomplete. One hint remains on
  screen, `h  help`, at the end of the bottom bar; `h` opens a modal listing every
  shortcut of the active tab, `h` or `esc` closes it, and it swallows keys while
  open so a key pressed at a list of keys never also acts on the table behind it.
  With the hints folded into the bar the Sessions chrome reaches six rows. `h` is
  now the help key on every tab, which costs the two undocumented vim aliases
  (`h`/`l`) that Settings carried for left/right — the arrows and `space` are what
  its hints have always advertised. The `a` binding is removed: a bare unmodified
  letter that rewrites `tui.toml` with no confirmation and no undo, while the same
  setting sits one tab away under a readable label with its value on screen
  ([sessions-chrome](docs/design/sessions-chrome.md), #493).
- The Sessions tab spends 7 rows on chrome instead of 10, and 3 full-width rules
  instead of 5. The summary row is deleted: its status counts were the exact
  aggregate of the STATUS column, `hidden 25` and `● ended 25` said the same
  thing twice on one line, and the active sort is readable from the arrow in the
  column header. What survives moves — the connection glyph to the end of the tab
  line, where the eye checks for state and where the corner never changes width;
  the sort, grouping and `hidden N` to the bottom bar, which now shares one width
  budget between its two halves. `hidden N` is shown only when something is
  hidden: it is the only thing on screen saying the list is filtered, and a
  permanent zero trains the eye to skip the place where the exception appears.
  The usage gauges are tightened against their percentage and their reset, giving
  up the alignment `%3.0f` used to keep — a deliberate reading trade
  ([sessions-chrome](docs/design/sessions-chrome.md), #492).
- The web dashboard's contract with the TUI is written down: **"mirror" binds
  content and hierarchy, not gestures.** The word appeared in three places and
  bound nothing, so the two clients drifted in both directions — the dashboard
  still drew the summary strip #492 deleted, and had none of the TUI's filter,
  grouping or idle-hiding — with no rule saying which side was the defect. The
  tempting justification, that a browser has rows to spare where a terminal does
  not, does not survive contact with the specification it appeals to:
  sessions-chrome.md calls the row count "the smaller half of the problem", and
  none of its three tests mentions screen size. So a divergence in *what is shown*
  is debt, while a difference in *how it is operated* — a keyboard modal against a
  mouse, an indicator against a button — is not. Where the boundary is checkable
  it is a test rather than a sentence (#544).

### Fixed

- ADR-0008 no longer states a consequence that #508 reversed. It said `compacting`
  clears a hook-posted `waiting` "like `working`/`thinking`" — true when it was
  written, and false since the reconciler became a deny list. The reason has
  nothing to do with compaction: to the watcher a running tool and a blocking
  permission prompt are the same frozen transcript, so nothing it infers from that
  silence may retract a `waiting` a hook established. The rule was an allow list,
  `stalled` fell through it when #256 added that status, and a permission prompt
  read as a hung tool for the rest of the session. An ADR is consulted before
  behaviour is changed, so one that argues clearly for a reversed position is how
  a fixed defect comes back: acting on that bullet would have rebuilt #508. The
  decision itself stands — the marker, the boundary close, the timeout cap, the
  rollup behaviour — and only the overtaken sentence carries an amendment (#559).

- Three small defects, none urgent, all real. The desktop notifier called `Start`
  and never `Wait`, so every attention transition and every raised call left an
  unreaped child until the TUI exited — on a tool meant to stay open all day. It
  is waited on in a goroutine now: `Run` would have fixed the zombie and blocked
  the update path instead. `Save` wrote `config.toml` at 0600, which applies on
  *creation* only, so a file left at 0644 by an older build kept the fleet token
  readable by every local account — the same case the daemon's database fixed in
  #526, and the comment there claimed the client already handled it. And neither
  HTTP listener set `IdleTimeout`, so a keep-alive connection was never closed;
  `WriteTimeout` stays deliberately unset, since it would sever the SSE stream at
  `/api/events`, and a test now pins that asymmetry so the next reader does not
  "complete" it (#560).

- The session id is stripped of control sequences like every other field the
  terminal renders. #529 cleaned twelve of them and stated in the code that the
  model never holds a string able to act on a terminal; it left out the one field
  nobody thinks of as text. The id arrives in a report body and the daemon checks
  only that it is non-empty, while the TUI prints it in full in the detail panel
  and uses it as the row's name for as long as a session has no title — so an
  `OSC 52` clipboard write or a screen clear travelled straight through. The
  fixtures guarding #529 all used `ID: "a"`, which is precisely why it went
  unseen (#540).
- The session-status specification describes the machine that exists. It stated
  twice that the watcher **cannot** observe `waiting` — true when it had only the
  transcript to read, where a permission prompt and a running tool are the same
  frozen file, and false since Claude Code began publishing a session registry
  that says so outright, reason included. The specification is the source of
  truth, so the honest reading of it was an instruction to delete the only way a
  machine with no hooks installed ever shows that a human is the blocker. The same
  document also claimed a session that stops reporting simply reads `ended`, the
  behaviour #284/#285 replaced and which its own § 1 already contradicted: silence
  on a machine whose watcher is still beating means gone, silence with no watcher
  at all means unobserved. The registry now appears in § 2 as what it is — a third
  signal, authoritative over the transcript heuristic where it covers a session —
  and `stale` has the detection row it never had. A test counts the statuses § 5
  fails to account for, since nothing checked that table before (#539).
- The web dashboard keeps observing once its event stream is live. It stopped
  asking for the session list the moment the stream connected, and `ended` and
  `stale` are not stored values — the daemon derives them from the clock every
  time the list is read, so that transition changes no field, publishes no event,
  and reached a client that only listened never. A machine whose watcher had died
  stayed `working` on screen for as long as the tab was left open, under a green
  `live` chip, while the TUI settled within five seconds; relative ages froze with
  it. The dashboard now refreshes on the same five-second tick the TUI has always
  used for exactly this reason, and carries the silence watchdog the TUI gained in
  #457: a suspended machine's connection dies without a FIN or an RST, so the read
  blocks for minutes and the reconnect path is unreachable without one. Asking
  often is not redrawing often — an unchanged fleet is left alone, so the scroll
  position, the selection and the keyboard focus survive the tick (#538).
- The web dashboard marks every session that calls for the operator. It decided
  for itself which ones those were and dropped `error`, so a session stuck on a
  529 was drawn like any working one, and the tab badge — the one number visible
  from another tab — counted `waiting` alone. The set is now the shared one, read
  from `internal/status` like the TUI's and the indicator's, with the Go guard
  that already existed for GNOME and had no twin for the dashboard (#466, #538).
- The last specifications that described a deleted screen are corrected, and the
  architecture map lists the GNOME indicator, which had never appeared in it
  despite being a shipped client with its own schema and release path. The two
  paragraphs that survived the earlier pass had both been missed the same way:
  their heading was updated and the prose beneath it was not — one still described
  the summary strip's status counts as the bottom bar's left half, the other a
  footer whose eleven hints are now a single `h help`. The GNOME preferences
  window also still promised notifications for "waiting" alone, where the schema
  beside it had already been corrected (#531).
- The terminal dashboard no longer relays control sequences to the terminal. A
  terminal does not merely display what it is sent: some sequences are commands to
  it — clear the screen, move the cursor, set the window title, write the
  clipboard. The strings vigie shows are transcript-derived, so a session's own
  title or activity could act on the operator's terminal at every refresh; the web
  dashboard has escaped its side since #161 for the same reason, and the terminal
  had nothing. Session text is now cleaned as it enters the client, before
  anything reads it — including the desktop-notification path, which is another
  program's input. Each removed character leaves a visible `?` rather than
  vanishing, so a title made only of them does not render as a blank line (#529).
- The `vigie_output_tokens_total` metric can no longer grow the daemon without
  limit. Its `model` label came straight from the report body, and Prometheus
  holds a counter per distinct label value for the process's lifetime and never
  frees one — so a client sending a different model name each time grew memory
  indefinitely, and one carrying a date or an id would have done it by accident.
  The label is now the model's family (`opus`, `sonnet`, `haiku`, `fable`), with
  anything unrecognized counted as `other` rather than dropped: a counter that
  silently under-counts is worse than a bucket whose name says it is a catch-all.
  Nothing visible is lost — the per-version breakdown lives in the daily stats
  table, which the Stats tab reads (#528).
- A malformed watch report can no longer freeze a session. The server tells its
  two informants apart by whether a report carries a status: one that does not is
  taken for a hook and believed on its word, its state stamped as coming from a
  reliable source — after which the watcher may never correct it. A report
  claiming to come from the watcher with no status fell into that branch, so the
  server invented `working` for it and locked the session there for the rest of
  its life. The watcher's whole contribution *is* the status it inferred, so an
  empty one says nothing; it is now refused. Hooks keep deriving theirs, which is
  correct — a `Stop` carries no status and means idle by definition — and the real
  watcher always sends one, so nothing legitimate is turned away (#527).
- The daemon's database is readable by its owner alone. It holds the fleet's
  shared token, and SQLite created it with the process umask — `-rw-r--r--` on a
  default one, so every local account on the host could read the secret and, with
  it, post reports or set the retention to `1ns` and wipe the session table. The
  client had always written its copy of the same secret at `0600`; the daemon,
  which holds the copy that matters, had no mode set at all. All three files are
  covered — the `-wal` carries committed pages — and an existing database is
  tightened when it is opened, not only a new one, since the operators who need
  this are the ones already running. The file is also created before SQLite can,
  so the database that persists between runs is never world-readable, not even
  for the moment between its creation and the chmod (#526).
- The specifications describe the screen that exists. `session-list.md` was marked
  **Accepted** while still specifying the `a` binding and the summary strip, both
  removed weeks ago — so an implementer following it rebuilt a screen the tests
  now reject. `sessions-chrome.md` contradicted itself about the same binding,
  and `tui-viewport.md` still drew the pre-reorganization stack. Corrected, along
  with the API table in `architecture.md`, which omitted the two routes deciding
  whether a watcher may write; `session-status.md`, which counted two statuses
  calling the operator where there are three plus a raised call, and declared
  `compacting` in one section then forgot it in two others; and the notification
  wording in the README and the GNOME schema, which both still promised
  "waiting" only. A test now compares the API table with the routes the server
  registers, so that table cannot drift again unnoticed (#511).
- The daemon validates what a report carries instead of trusting it. An unknown
  event fell through to `working` **and** was stamped hook-owned, which the
  watcher could then no longer retract — the #201 failure mode, reachable from a
  single malformed request. Events and statuses are now checked against the
  vocabularies that exist. The `/rc` resume URL is validated at ingestion and only
  `https` is stored: the dashboard puts it in an `href`, and HTML escaping stops
  an attribute being broken out of, not a `javascript:` scheme from being
  followed. And the import barrier that keeps the server out of the client binary
  named a package path that does not exist, so it matched nothing — corrected, and
  widened from three client packages to all eight (#515).
- `vigie init` no longer claims to install hooks. The watcher has owned them
  since ADR-0009, and #415 removed hook installation from `init` — but `vigie
  help` and the README's command table still described the old contract, and the
  README contradicted itself fifty lines later. An operator setting up a machine
  reads the table, runs `init`, and believes the hooks are in place; on a machine
  running no watcher they are not, and nothing reports. The help and the table are
  corrected, ADR-0008's two stale instructions with them, and ADR-0009 is amended
  in place to say that #415 carried its decision further rather than reversing it.
  A test now compares the README's command table with what the CLI actually
  offers, so the two cannot drift apart again unnoticed (#510).
- The dashboard redraws when a session's effort, context or permission mode
  changes. The signature that decides whether a report is worth an SSE event
  covered nineteen fields and omitted four the dashboard renders, and the web
  client stops polling once the stream is live — so a session switching to plan
  mode, changing reasoning effort or filling its context window never redrew. The
  TUI hid it by refetching every five seconds regardless. A dashboard that has
  stopped updating one column looks exactly like one where nothing changed. The
  four are covered, and a test over the API view now fails when a field is added
  without being either covered or excluded with a reason — the list is what
  drifted, because nothing tied it to what it was meant to cover (#514).
- A hook installed from a path containing a space now reports. The command
  written into `settings.json` was built by concatenation and never quoted, so a
  binary at `/home/me/My Tools/vigie` produced a command the shell split — it ran
  `/home/me/My` and failed. Nothing said so: the hook was installed, the file
  looked right, `vigie hooks install` reported success, and no event ever arrived,
  which makes `waiting` — the one status only a hook can see — permanently
  invisible. Both the binary path and the config override are quoted now, and the
  matching that recognizes vigie's own hook legs accepts the quoted form as well
  as the bare one, so legs installed before this are replaced rather than
  duplicated (#513).
- The Machines tab no longer hides five statuses out of nine. Its per-machine
  counts came from a hand-written switch covering `working`, `waiting`, `idle` and
  `ended`; `thinking`, `compacting`, `stalled`, `error` and `stale` fell through it
  while still counting towards the session total, so a machine running three
  stalled sessions rendered "3 sessions, 0 everywhere". Two of the three statuses
  that call the operator were among the invisible ones, and nothing looked wrong
  because the totals still added up. Counts now come from the status vocabulary
  itself, including a status this build has never heard of, and anything without a
  column of its own is spelled out at the end of the row. A fleet whose statuses
  all have columns renders exactly as before (#509).
- A report can no longer overwrite another one's. The server read a session,
  merged the report in Go and wrote it back, with nothing holding the row in
  between — and SQLite serializes writes, not a read-modify-write cycle. Measured
  on two concurrent cycles, the overlap is about 300 microseconds wide: both reads
  land before either write, so the second write is computed from a state the first
  has already replaced. Every reconciliation rule in the server is "given the
  current status and its source, decide", so reading a stale current made the
  outcome depend on commit order rather than on the rules. The cycle is now one
  atomic step. Without it, sixteen concurrent reports adding ten tokens each left
  twenty tokens instead of a hundred and sixty (#512).
- A session blocked on a permission prompt no longer reads `stalled`. The
  `Notification` hook reports `waiting` — the one status only a hook can see — but
  the prompt freezes the transcript on an unanswered tool call, which is exactly
  the shape the watcher reads as a hung tool, and about 45 seconds later it said
  so. It did not go quiet, it named the wrong cause: both statuses call the
  operator, so the queue looked the same size while sending them to look at a tool
  instead of answering the prompt waiting for them — and `stalled` is
  watcher-owned, so the `waiting` did not come back. The guard that defends a
  hook-set `waiting` was an allow list of three statuses, written before `stalled`
  existed; it is now a deny list of the two the watcher establishes positively
  (`error`, `ended`), so a status added later is held by default
  ([session-status](docs/design/session-status.md) § 3, #508).
- The README's animation shows the Sessions tab the TUI actually renders. It still
  drew the status-count row and its rule, the old gauge spacing, and no state
  pill — a screen deleted three pull requests ago, and the first thing a visitor
  sees. Five tests on the asset passed throughout, because every one of them
  compares the four rendered files with each other and with the template they came
  from: internal consistency, never truth. A new guard renders a fleet through the
  real TUI and compares the chrome against the drawing — how many rows frame the
  table, and which chrome elements are present or absent — and it fails from both
  sides, whether the asset falls behind or the TUI grows a row. It states in the
  test what it does not cover: the asset is a drawing, not a screenshot, so its
  data, colors and layout are its own (#505).
- `TestWatcherRunning` no longer fails at random on CI. The probe it starts —
  a copy of `sh` named like the watcher — ran `sleep 300; :`, so the shell forked;
  and between that fork and its `execve` the child carries the parent's `comm`
  *and* the parent's `cmdline`, because the kernel copies both and only `execve`
  replaces them. For a few microseconds there was a process that was not the
  probe's pid, was named like the probe, and had `watch` in its cmdline — exactly
  what the scan looks for, so the test's own probe impersonated a second watcher.
  It now runs `read line` with its stdin held open: `read` is a builtin, so the
  shell blocks without forking and the window is gone by construction. A guard
  fails the test if the probe ever forks again. The earlier attempt at this (a
  per-process probe name) addressed a cause that was not the one (#476).
- A session that calls you always blinks. The `blink` preference is removed, not
  exposed: it was the one setting whose "off" state was invisible — no control in
  the Settings tab, nothing on screen to say the marker was muted — so an operator
  could sit in front of a silent call with no way to learn why, or to undo it from
  inside vigie. An accelerator the operator cannot see is worse than no
  accelerator. A `blink` key left in an existing `tui.toml` is ignored and dropped
  on the next save, which is what clears the symptom. `call_marker` stays: it is a
  font escape hatch, not a comfort setting
  ([ADR-0010](docs/adr/0010-session-raised-operator-call.md), #490).
- The summary strip keeps the view state on a narrow terminal. Its two halves did
  not share a width budget: the left block (status counts, totals) was fitted
  against the full terminal width, after which there was no room for the right one
  and it was dropped whole. Below ~140 columns that cost `sort`, `group`, `hidden`
  and the server-connection glyph at once, with nothing left on screen to say they
  had been cut — and the glyph is the only permanent sign the client is still
  reaching the server. The right block's width is now reserved first and the left
  block fitted into what remains, dropping its decoration as it already did. Status
  counts are dropped whole rather than sliced, so `ended 25` can no longer render
  as `ended 2`, and an ellipsis marks a shortened list
  ([session-list](docs/design/session-list.md), #486).
- The key-hint footer costs one row at any width. It was wrapped to the terminal
  width rather than fitted, and the Sessions hints need 134 columns — so every
  narrower terminal spent an extra row on it, on every frame, charged to the
  session table. Hints are now dropped whole, least essential first, with an
  ellipsis marking the cut; `q quit` is never dropped
  ([tui-viewport](docs/design/tui-viewport.md), #487).
- The deployment guide documented Prometheus metrics that do not exist. It said
  they were namespaced `fleet_*`, with a `fleet_sessions` gauge; the server has
  emitted `vigie_*` since the rename. An operator who built scrape rules or alerts
  from that paragraph collected nothing and was never told — a query for a series
  that does not exist returns an empty result, not an error. The prose is
  corrected, and a test now checks every metric named in the docs or in the
  shipped Grafana dashboard against the ones the code registers, so a stale
  prefix, a renamed metric or a deleted one all fail the build. The dashboard
  itself was already correct ([deployment](docs/deployment.md), #478).
- The old brand is gone from the identifiers that ship to users: the GNOME
  indicator's class and its four `cf-*` CSS classes, the dashboard's `cf_token`
  and `cf_columns` storage keys, and three package doc comments. The two storage
  keys hold live state, so they are carried over on first read rather than
  dropped — nobody is signed out of the dashboard and no column layout is lost.
  The `FLEET_CONFIG` fallback and the `~/.config/claude-fleet` read-fallback stay:
  they are a deliberate migration path, not a leftover (#478).
- A tool call that never got its result no longer pins a session to `stalled` for
  the rest of its life. The `tool_use`↔`tool_result` pairing that detects a hung
  tool (#256) dropped an entry on one event only — the matching result — so a
  session killed while a tool was in flight kept that call pending forever and
  read `stalled` at every pause between turns, long after it had gone back to
  working normally. `stalled` is one of the signals that call the operator, and
  this one was unclearable: no action on the session removes it, and vigie is
  read-only towards sessions. The pairing is now scoped to the turn — a prompt the
  operator typed closes every older unresolved call, since a call from before the
  prompt cannot be what the current turn is parked on. Lines Claude Code injects
  itself (system reminders, skill preambles, the "Continue from where you left
  off." resume) are excluded: they land in the middle of live tool calls, and
  closing on them would break stall detection outright
  ([session-status](docs/design/session-status.md) § 2, #483).
- A preferences file that cannot be read is kept, not replaced. `loadPrefs` fell
  back to the defaults on a read or parse error, and the next preference keystroke
  wrote those defaults over the file — turning a recoverable problem into a lost
  configuration, with nothing said anywhere. An empty file was worse: it parses
  cleanly into zero values, so `hide_ended` flipped and the column layout vanished
  while everything looked normal. Unreadable, empty and unparsable are now told
  apart from absent; the file is left untouched, the session runs on defaults, and
  the Settings tab says so. The write also goes through a temp file renamed over
  the target, as `internal/install` already did for `settings.json`, so a TUI
  killed mid-save can no longer leave a truncated file where the real one was
  (#480).

- Running the tests no longer overwrites the operator's own TUI preferences.
  `TestGroupToggleCycles` built a zero-value model and sent `g`, which saves the
  view preferences — into the **real** `~/.config/vigie/tui.toml`, because that
  test never redirected `XDG_CONFIG_HOME`. A zero-value `prefs` is not
  `defaultPrefs()`, so the file came back with `column_order = []` and the saved
  column layout was gone. A second leak of the same kind wrote a phantom
  `s1.json` into the live `~/.local/state/vigie/sessions/`, the directory the
  watcher reads. Both packages now isolate their home directory for the whole test
  run, so a future test cannot leak by forgetting a redirect, and a guard fails if
  that isolation is ever removed. Measured before and after: the suite wrote two
  files into a stand-in home, and now writes none (#479).

- The GNOME indicator now raises a badge and a notification for every signal that
  calls for the operator, not only `waiting`. A stalled turn — a tool that hung and
  will not resolve itself — raised nothing, and a session's own call, the headline
  of 0.5.0, did not exist for the extension at all: on a machine where the operator
  watches the top bar rather than a terminal, that feature was invisible. The
  notification also says *why*, since a stalled turn, an API error and a raised
  call want different things and one wording for all three misinforms. The set of
  statuses that warrant an interruption is now declared once in `internal/status`
  and shared, so the indicator and the TUI cannot disagree about when to interrupt
  you (#466).

- Sorting by status now places every status, in both dashboards. Only five of the
  nine were ranked: in the TUI the other four fell to an unranked default that put
  them **below `ended`**, so a session hitting an API error sorted under one that
  was over. In the web dashboard `compacting` was missing from the rank table, so
  the comparator returned `NaN` — which does not order badly, it stops ordering:
  the table came out with an ended session first. The nine are now ranked once, in
  [the design document](docs/design/session-list.md) § 2.1 and in `internal/status`,
  and both clients read from it. An unknown status sorts last rather than first,
  since it is the one the build can say least about. #423 locked which statuses
  exist; this locks how they sort, which its own text had flagged as the remaining
  gap (#464).

- The event stream now notices its own death in seconds rather than minutes. A
  suspended machine's connection dies without a FIN or an RST, so the client's read
  blocked on a socket that would never deliver another byte, until the OS gave up
  its keepalive probes. The reconnect loop was correct all along and simply never
  ran, because the function it guarded had not returned. The daemon now sends a
  keep-alive comment every 10 s, and the client gives up after 30 s of silence —
  three missed beats — and reconnects. The connection indicator also stops claiming
  a live stream while polls are failing: that observation was made before the
  machine went to sleep, and a failing poll is present-tense proof the server is out
  of reach (#457).

- A failed poll no longer blanks the sessions table. Resuming a laptop from suspend
  produced `error: … context deadline exceeded` and **no sessions at all**, for the
  minutes the connection took to come back. The sessions were never lost — the
  model kept them — the view was discarding data it still had. The table now stays
  on screen with a notice naming the failure, and the error stands alone only when
  there is nothing to fall back on. This was the last panel still doing what the
  previous entry fixed everywhere else, and the one that matters most (#456).

- A panel now says when it is showing figures it could not refresh. The TUI fetches
  seven things from the daemon; only a sessions failure ever reached the operator.
  For the other six the error was discarded and the previous values stayed on
  screen — right in itself, since blanking a panel on a transient blip would be
  worse, but presented as current. A panel failing to refresh for an hour looked
  exactly like one that was up to date, and an endpoint that had never answered
  looked like "no data". Stats, Machines and Settings now carry a notice above
  their figures, the bottom strip carries a mark where a sentence would not fit,
  and a successful refresh clears it. The figures themselves are untouched: they
  remain the last thing known (#449).

- An abandoned metadata file is no longer reported as a session. Claude Code writes
  a sidecar next to each conversation — title, mode, permission mode, agent name
  and color — under the project's working directory, so a renamed or moved project
  leaves one behind holding no exchange at all. The watcher reported it as a
  session, with a status: 8 of 148 transcripts on one machine are such files. The
  filter is not "no conversation", because a session you have just started and not
  typed into has none either and must stay visible; it is "no conversation **and**
  not in Claude Code's session registry", the liveness source vigie already reads
  every scan. A started session is registered from the start, and an ended
  conversation still has its turns (#448).

- A synthetic assistant line no longer becomes the session's model. Claude Code
  writes `"model":"<synthetic>"` on lines it generates itself instead of receiving
  from the API; nothing filtered it, so the session showed `<synthetic>` as its
  model in the TUI and the dashboard until the next real turn — and because the
  daily rollups key on the session's model, every token produced meanwhile was
  attributed to a bucket that is not a model. One production day held
  `<synthetic> / output_tokens = 12879`: real output taken from the real model,
  and `stats_daily` is never pruned, so it stayed. The parser now keeps the last
  real model. The test is the bracketed marker rather than the API-error flag,
  which would have been wrong: of the nine synthetic lines found across one
  machine's transcripts, only five were flagged as errors — the rest are ordinary
  ("No response requested."). An unknown model remains its own `""` bucket, which
  cannot distort token figures: output only comes from assistant lines and those
  name a model, verified across 314 transcripts where all 18 with no model carry
  zero tokens ([design](docs/design/token-rollup.md), #433).

- Daily token stats could be inflated by orders of magnitude, permanently. The
  rollup counted the *growth* of the session's own token counter, so any time that
  counter regressed, the next report added the session's **entire lifetime total**
  again — and `stats_daily` is never pruned and never recomputed, so the wrong
  figure poisoned the Week/Month/Year/Total aggregates for good. One production day
  held 61 051 295 773 output tokens where the session reported 2 713 408: the whole
  total re-added on nearly every 2 s scan for half a day. Two causes are fixed and
  both are reproduced by tests: **one session written to two transcript files**
  (Claude Code stores them under the working directory, so a renamed or moved
  project yields two files with the same session id — the watcher now sends one
  report per session, keeping the live file's), and **resuming a session older than
  the retention window** (its row is gone, so the daemon re-counted it from zero).
  The rollup now counts against a per-session high-water mark held outside the
  session lifecycle, so a regression contributes nothing whatever its cause, and
  real growth counts exactly once. `vigied stats-repair` corrects a bucket that was
  already corrupted — daily stats cannot be recomputed, so the figure is the
  operator's decision, never guessed
  ([design](docs/design/token-rollup.md), #432).

- The GNOME extension no longer hides sessions. Its menu was built by iterating a
  hand-written list of four statuses, so a session that was `thinking`,
  `compacting`, `stalled`, `error` or `stale` matched no group and was dropped —
  and since the "No sessions" placeholder only appears when the list is genuinely
  empty, a fleet of stalled sessions rendered a menu that merely looked empty.
  `stalled` — a turn parked on a hung tool — is among the states most worth a look.
  The menu now renders every status, and appends any status it does not recognize
  rather than dropping it, so one added later degrades to "unstyled" instead of
  "invisible" (#422).
- The `vigie_sessions` gauge counts `compacting` sessions. It tallied every status
  but emitted a series only for those in its own list, which never gained
  `compacting` when that status was added — so those sessions were counted and
  then discarded, and the gauge's total did not match the number of sessions
  (#421).
- The nine session statuses are now declared once and checked everywhere. The list
  was hand-copied into four consumers, each incomplete in a different way, and
  nothing detected the divergence — which is how the two defects above went
  unnoticed for two releases. A test now reads the vocabulary from
  [the design document](docs/design/session-status.md) and fails on any copy that
  drifts, naming the entries that differ (#423).

- The reporting hook no longer re-reads the whole transcript at the end of every
  turn on a watched machine. `Stop` parsed the file from byte 0 to collect six
  fields, inside a hook Claude Code waits on with a 5 s timeout — and transcripts
  are append-only, so the cost grew with the session and never came back down.
  Measured over the 357 transcripts of one machine: 0.2 MB at the median, but
  62 MB at p99 (1.2 s per turn) and 585 MB at the maximum, where the parse took
  **11.1 s** — past the timeout, so the report was lost *and* the session stalled
  5 s after every turn. A local watcher already parses the same file incrementally
  every ~2 s and reports a superset of those fields, so the hook now defers to it
  and sends the report without them; the server already keeps the last known value
  for any field a report omits. `vigie watch` publishes the mark the hook reads
  (`~/.local/state/vigie/watcher`), refreshed on each scan and trusted for 15 s, so
  a watcher that stops — or one gone inert on a version drift, which stops scanning
  — makes hooks resume reading within one window. A machine running no watcher is
  unchanged: it has no other source for these fields
  ([design](docs/design/transcript-reads.md), #420).

## [0.5.0] - 2026-08-13

### Added

- `vigie` now installs a personal Agent Skill (`~/.claude/skills/vigie-call/`) so
  Claude knows the `vigie call` command exists without any per-project setup — the
  whole feature rests on the command actually being run when you ask to be told.
  It is written by `vigie init` and `vigie hooks install`, refreshed by
  `vigie watch` at startup so an install predating a release cannot keep a stale
  description, and removed by `vigie hooks uninstall`. The production leg alone
  owns it: a dev leg touches no production artefact. The skill states plainly that
  the call is **best-effort** — if Claude does not run it, nothing is raised and
  the session reads exactly as it does today
  ([design](docs/design/call-discoverability.md), #391).
- The web dashboard surfaces a session's call with the same grammar as the TUI —
  the marker lives in the status, never in the Detail cell. The dot inside the
  status pill **pulses** (a soft CSS fade, where the terminal is limited to two
  hard states), the pill keeps its status color and label because a calling
  session is still `idle`, the row takes the existing attention left-border plus a
  faint tint of the same color, and the call message fills the Detail cell in that
  color. No new color: everything derives from `--st`. `prefers-reduced-motion:
  reduce` stops the pulse, and a call counter leads the summary strip when
  non-zero ([ADR-0010](docs/adr/0010-session-raised-operator-call.md), #390).
- The TUI now surfaces a session's call by **motion**: its status dot blinks in
  its own status color, at 1 Hz (inside WCAG 2.3.1's three-flashes-per-second
  ceiling), and the call message takes the `DETAIL` cell in that same color. No new
  glyph, color or column — the dot is the one element in a row that can be
  animated without destroying information, since the status word stays readable
  beside it. A `● call N` counter leads the summary strip when non-zero, a raised
  call reuses the desktop notification, and it jumps ahead of the inferred
  attention states in the `n` queue — a call is explicit where `waiting` is a
  deduction. The animation tick exists only while something is actually calling;
  the ambient poll stays at 5 s. Two preferences in `tui.toml`: `blink = false`
  stops the animation, and `call_marker` changes the glyph for fonts that lack it
  — a marker wider than one terminal cell is rejected rather than allowed to shift
  every column to its right ([ADR-0010](docs/adr/0010-session-raised-operator-call.md), #389).
- A session can now raise an explicit **call** for the operator: ask for it in
  plain language ("when you're finished, tell me in vigie") and Claude runs
  `vigie call "backfill done — 12k rows"` at the end of its turn. vigie surfaces
  the call until work resumes in that session. The call is set **and cleared by
  the session** (`UserPromptSubmit`, `SessionEnd`), so no action on vigie is ever
  required — it is an observed signal like status, not operator handling state,
  which is what keeps it on the right side of
  [ADR-0007](docs/adr/0007-read-only-to-operator.md)
  ([ADR-0010](docs/adr/0010-session-raised-operator-call.md), #388). It is
  orthogonal to status — a calling session keeps whatever status it has — and the
  message is optional. Like the hooks it is fire-and-forget: it can never fail a
  session. Rendering lands separately (TUI #389, web #390).
- The sessions table now scrolls within a vertical viewport instead of spilling
  off the bottom of the terminal. It tracks the terminal height (previously only
  width was honored), keeps the tab bar, summary, column header, and usage/footer
  pinned, and scrolls only the row band — continuous, cursor-driven, htop/k9s
  style, with a 2-row look-ahead margin and a `rows a–b / n` indicator shown only
  when the list overflows. Grouped views keep the current group's header pinned;
  the detail panel scrolls the same way ([design](docs/design/tui-viewport.md),
  #378).

### Changed

- The `DOING` column is now `DETAIL`, in the TUI and the web dashboard alike. The
  name no longer described its contents: three of the five things it carries are
  not actions (a permission prompt's subject, `shell`, a call message) and a
  fourth is the negation of one (`interrupted`). It also removes a real ambiguity
  — a *different* column was already called `ACT`/`Activity` (the token
  sparkline). The API field follows: `GET /api/sessions` now returns `detail`
  instead of `activity`. That is a contract change, and it is coordinated rather
  than silent: the TUI and the daemon are already version-locked to each other
  (the startup preflight), the web dashboard is served by the daemon itself, and
  the report endpoint still accepts the old `activity` field from a hook reporter
  that predates the rename — the one client deliberately exempt from the version
  gate. A saved column layout is migrated, so a renamed column keeps its position
  and stays hidden if you had hidden it (#393).
- A watcher whose build does not match the daemon can no longer write session
  state. Enforcement lives in the daemon — the watcher's build already travels in
  every report (#356), and a rule applied only by the client can be skipped by
  exactly the outdated client it must stop — which closes the gap where a machine
  running a watcher but never a TUI drifted unchecked. A refused report answers
  `409` and writes nothing, while the machine and its faulty build stay visible in
  `GET /api/watcher` and the Machines tab, so the operator can see what to
  upgrade. The watcher goes **inert** rather than exiting (the packaged unit uses
  `Restart=on-failure`, so exiting would crash-loop): it logs the drift once,
  retries a single report every 60 s, and resumes on its own once the builds
  realign. Hook reports stay ungated on purpose — they run inside the operator's
  Claude session ([design](docs/design/version-consistency.md), #384).
- `vigie init` now writes the config and nothing else. The reporting hooks and the
  call skill had **three** writers — `init`, `vigie hooks install` and `vigie
  watch` — and only the watcher's copy self-heals, which is the whole point of
  [ADR-0009](docs/adr/0009-watcher-managed-hooks.md). They now have one owner: the
  watcher installs them at startup and keeps them matching the running binary.
  `init` ends by saying what is left to do — start the watcher, **or restart it**
  if one is already running, since it reads the config only at startup. This also
  removes a trap: `init` used to rewrite the *production* hooks even when
  `VIGIE_CONFIG` pointed at a dev leg. A machine that runs no watcher still wires
  itself with `vigie hooks install`, and `vigie hooks uninstall` removes both.
  **Breaking:** `vigie init --uninstall` is removed — it undid something `init` no
  longer does. `vigie hooks uninstall` replaces it and is strictly more complete,
  since it also removes the call skill (#415).
- `vigie init` now **asks** for the server URL, the token and this machine's name,
  and takes **no flags at all**. The token is read **without echo**, so the shared
  secret no longer lands in the shell history of every machine — the same reason it
  has no place in a systemd unit. The machine name defaults to the hostname, which
  is right most of the time and wrong exactly where it matters: a container's is a
  random hash, and the prompt is where you can correct it. Without a terminal it
  fails with a clear message rather than blocking on a question nobody can answer
  (#407, #415).

### Fixed

- A machine whose watcher is running but currently has **no session to report** no
  longer reads as having no watcher. Liveness was a side effect of session data —
  the server only refreshed a machine's heartbeat while handling a session report —
  so a machine with nothing open (or nothing newer than `--max-age`) silently
  dropped out of `GET /api/watcher`, showed as "hooks only" in the Machines tab,
  and made the TUI preflight refuse to start while blaming the server. The watcher
  now claims liveness on its own, every 5 s, over a dedicated
  `POST /api/watcher/heartbeat` that is independent of sessions
  ([design](docs/design/watcher-liveness.md), #386). That heartbeat also carries
  the version verdict, replacing the 60 s report-retry probe from #384 — which
  could never work on the machine this fixes, since a drifted watcher with no
  sessions had no report to probe with.
- Desktop notifications could be **impossible with nothing saying so**. The TUI
  assumed it had focus until the terminal said otherwise, so a terminal or
  multiplexer that never reports focus events suppressed every notification
  forever — correct settings, working desktop, and nothing ever arriving. Focus is
  now three-valued and *not knowing* no longer counts as "you are watching": a
  notification while you are already looking is a small annoyance, never receiving
  one is a broken feature. The Settings tab also now says **why** notifications
  cannot be delivered — `on — notify-send not installed`, `on — no graphical
  session` — instead of showing a cheerful `on` on a machine where nothing can
  work (#411).

## [0.4.1] - 2026-08-08

### Fixed

- The daemon no longer returns intermittent `500`s on session and usage reports
  under load. `busy_timeout` and `foreign_keys` are per-connection SQLite state
  but were set on only the first pooled connection, so every other connection the
  pool opened had `busy_timeout=0` and failed a contended write immediately with
  `SQLITE_BUSY` — and the watcher writes every session (plus usage) every ~2 s.
  The pragmas now travel in the DSN, so the driver applies them to every
  connection and contending writers wait instead of erroring (#372).
- The TUI startup preflight no longer reports a running watcher as down. A stale
  server heartbeat is a failed round-trip, not proof the watcher is dead — a
  just-restarted watcher or an unreachable server looks identical. The preflight
  now cross-checks a local `/proc` liveness signal: it says "watcher not running,
  start it" only when no local watcher process exists, and otherwise points at the
  server/connectivity and says to retry (#371).

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

[Unreleased]: https://github.com/haribo/claude-vigie/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/haribo/claude-vigie/compare/v0.4.1...v0.5.0
[0.4.1]: https://github.com/haribo/claude-vigie/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/haribo/claude-vigie/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/haribo/claude-vigie/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/haribo/claude-vigie/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/haribo/claude-vigie/releases/tag/v0.1.0
