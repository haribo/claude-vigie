// Pure helpers for the dashboard: formatting, escaping, and the column-layout
// rules. Nothing here touches the DOM, `localStorage`, or the network, which is
// the whole point — app.js keeps the parts that do, and these can be exercised
// under node (#430).
//
// `fullColOrder` and `colHidden` take the column set as an argument rather than
// reading a module-level constant, so a test can state the columns it means
// instead of inheriting whatever the dashboard happens to define.

export function esc(s) {
  return String(s == null ? "" : s).replaceAll("&", "&amp;").replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;").replaceAll('"', "&quot;").replaceAll("'", "&#39;");
}

export const dash = (v) => (v === "" || v == null) ? "-" : v;
export const trim = (x) => x.toFixed(1).replace(/\.0$/, "");

// A session-raised call (ADR-0010). call_at is what marks it: the message is
// optional, and a call with none is still a call.
export function hasCall(s) { return Boolean(s && s.call_at); }

// detailText is the Detail cell's content. What it shows and in what order — a
// raised call first, then the API error code, then the activity — is the daemon's
// (ADR-0011, #618); the vocabulary of error labels went with it, so this side no
// longer holds a copy to drift. What stays is the escaping, which is this
// client's own concern: the daemon serves text, and a browser is not a terminal.
export function detailText(s) {
  return esc(s && s.detail_text ? s.detail_text : "-");
}

// humanTokens is the twin of `humanizeTokens` in internal/tui/render.go, proved
// against test/fixtures/format-cases.json (#619).
//
// Integer arithmetic, matching the Go side character for character in intent: a
// float divide followed by a rounding rounds twice, and the two languages land on
// different sides of it. 1150 tokens read `1.2k` in the terminal and `1.1k` here
// for exactly that reason, and so did every count ending in 50.
//
// The decimal is always kept — `1.0k`, not `1k` — because the terminal column is
// aligned on a character grid and a width that changes with the value breaks it.
// The two clients show one figure or they show two.
export function humanTokens(n) {
  n = Number(n) || 0;
  if (n >= 1e6) return oneDecimal(n, 1e6) + "M";
  if (n >= 1e3) return oneDecimal(n, 1e3) + "k";
  return String(n);
}

function oneDecimal(n, unit) {
  const t = Math.floor((n + unit / 20) / (unit / 10));
  return `${Math.floor(t / 10)}.${t % 10}`;
}
export function ageSec(rfc) { const t = Date.parse(rfc); return Number.isNaN(t) ? Infinity : Math.max(0, (Date.now() - t) / 1000); }

export function relAge(rfc) {
  const s = ageSec(rfc); if (!Number.isFinite(s)) return "-";
  if (s < 60) return Math.floor(s) + "s";
  const m = Math.floor(s / 60); if (m < 60) return m + "m";
  const h = Math.floor(m / 60); if (h < 24) return h + "h";
  return Math.floor(h / 24) + "d";
}

export function relResetHint(rfc) {
  const t = Date.parse(rfc); if (Number.isNaN(t)) return "";
  let s = Math.floor((t - Date.now()) / 1000); if (s <= 0) return "resets soon";
  const d = Math.floor(s / 86400); s -= d * 86400; const h = Math.floor(s / 3600); s -= h * 3600; const m = Math.floor(s / 60);
  if (d) return `resets in ${d}d ${h}h`; if (h) return `resets in ${h}h ${m}m`; return `resets in ${m}m`;
}

export const totalTokens = (u) => (u.input_tokens || 0) + (u.output_tokens || 0) + (u.cache_creation_tokens || 0) + (u.cache_read_tokens || 0);

export function sparkSVG(data, w = 72, h = 18) {
  if (!data || !data.length || Math.max(...data) === 0) return '<span class="faint">—</span>';
  const max = Math.max(...data, 1), n = data.length, step = n > 1 ? w / (n - 1) : 0;
  const pts = data.map((d, i) => [i * step, h - (d / max) * (h - 3) - 1.5]);
  const line = pts.map((p) => `${p[0].toFixed(1)},${p[1].toFixed(1)}`).join(" ");
  const area = `0,${h} ${line} ${w},${h}`;
  const [ex, ey] = pts[pts.length - 1];
  return `<svg class="spark" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}" aria-hidden="true">
    <polygon points="${area}" fill="var(--st)" opacity="0.12"/>
    <polyline points="${line}" fill="none" stroke="var(--st)" stroke-width="1.5" stroke-linejoin="round" stroke-linecap="round"/>
    <circle cx="${ex.toFixed(1)}" cy="${ey.toFixed(1)}" r="2" fill="var(--st)"/></svg>`;
}

// adoptLegacyKey moves a value saved under a pre-rename localStorage key to its
// current name, once, and returns what is now stored there.
//
// The dashboard's keys carried the old brand (`cf_token`, `cf_columns`). Renaming
// them outright is not a rename at all from the operator's side: those keys hold
// live state, so every open dashboard would have been signed out and every column
// layout lost — for a cosmetic gain. Reading the old key once, under the new name,
// makes the rename invisible. The new key wins if both exist, so a value written
// since the upgrade is never rolled back by a stale leftover (#478).
export function adoptLegacyKey(storage, oldKey, newKey) {
  const current = storage.getItem(newKey);
  if (current !== null) {
    storage.removeItem(oldKey); // the new key is authoritative; drop the leftover
    return current;
  }
  const legacy = storage.getItem(oldKey);
  if (legacy === null) return null;
  storage.setItem(newKey, legacy);
  storage.removeItem(oldKey);
  return legacy;
}

// LEGACY_COLS maps a key saved under an older name to its current one. Without
// it, a layout saved before a rename silently loses that column.
export const LEGACY_COLS = { doing: "detail" };

// V1_COLUMN_KEYS is the one-shot remap from the pre-#550 dashboard key set to the
// TUI's. It is deliberately NOT merged into LEGACY_COLS, which is applied on
// every layout read: this map is not idempotent — it renames `activity` to `act`
// *and* `act` to `open`, so a second pass would turn the freshly-migrated
// sparkline into the detail button. It runs exactly once, when a v1 layout is
// carried over to the v2 storage key, and the v1 key is then removed.
export const V1_COLUMN_KEYS = { doing: "detail", project: "dir", tokens: "total", activity: "act", act: "open" };

export function migrateV1Columns(keys) {
  const seen = new Set();
  return (keys || []).map((k) => V1_COLUMN_KEYS[k] || k).filter((k) => !seen.has(k) && seen.add(k));
}

export function migrateKeys(keys) {
  const seen = new Set();
  return keys.map((k) => LEGACY_COLS[k] || k).filter((k) => !seen.has(k) && seen.add(k));
}

// fullColOrder is every column key in display order: the saved order (unknown
// dropped), with any column missing from it appended in the built-in order.
export function fullColOrder(order, colKeys) {
  const known = new Set(colKeys), seen = new Set(), out = [];
  (order || []).forEach((k) => { if (known.has(k) && !seen.has(k)) { out.push(k); seen.add(k); } });
  colKeys.forEach((k) => { if (!seen.has(k)) { out.push(k); seen.add(k); } });
  return out;
}

export function colHidden(hidden, key, mandatory) { return !mandatory.has(key) && hidden.includes(key); }

// Sort order for the status column, lower first. Kept identical to
// docs/design/session-list.md § 2.1 and internal/status.Order — a Go test reads
// this literal and fails on drift.
//
// It used to be an object with eight of the nine statuses, so `RANK[status]` was
// `undefined` for `compacting` and the comparator returned `NaN`. A NaN
// comparator does not order badly, it stops ordering: the table came out with an
// `ended` session first (#464).
// rank is where a status sorts, lower first. The daemon sends it (ADR-0011,
// #617): the order used to be transcribed here and into the GNOME indicator, and
// #464 is what that cost — four statuses nobody had ranked produced a NaN
// comparator, which does not sort badly, it stops sorting.
export function rank(session) {
  const r = session == null ? null : session.rank;
  return typeof r === "number" ? r : Number.MAX_SAFE_INTEGER;
}

// fuzzyMatch reports whether pattern appears in text as a subsequence, ignoring
// case: every character of the pattern in order, gaps allowed. `wapp` matches
// `web-app`. It is deliberately not a substring match and not a regex.
//
// This is a hand port of `fuzzyMatch` in internal/tui/model.go, and a copied rule
// is what #421, #422 and #466 all were. So it is not trusted: a shared fixture
// (test/fixtures/fuzzy-cases.json) is run against both implementations, the Go
// one in internal/tui and this one under node. Iterating the lowered string by
// code point matches Go's `range` over runes.
//
// The one place the two languages can still disagree is case folding outside
// Latin — Go's strings.ToLower and JS toLowerCase differ on a handful of scripts.
// The fixture stays inside what a session title, a machine name or a branch
// realistically contains.
export function fuzzyMatch(pattern, text) {
  const pr = [...String(pattern == null ? "" : pattern).toLowerCase()];
  let pi = 0;
  for (const tr of String(text == null ? "" : text).toLowerCase()) {
    if (pi < pr.length && tr === pr[pi]) pi++;
  }
  return pi === pr.length;
}

// sessionHaystack is the text the filter searches — the twin of the Go function
// of the same name. Field order and the single spaces between them are part of
// it: a pattern may span two fields, so a different join is a different filter.
export function sessionHaystack(s) {
  if (!s) return "";
  const f = (v) => (v == null ? "" : String(v));
  return [f(s.name), f(s.machine), f(s.project), f(s.git_branch), f(s.status)].join(" ");
}

// matchesFilter applies the active filter to one session. `rc` as the whole
// pattern is a special token selecting remote-controlled sessions rather than a
// text match — `internal/tui/sessionsview.go` does the same, and an operator who
// learns it in one window must find it in the other.
export function matchesFilter(s, filter) {
  if (!filter) return true;
  if (filter.toLowerCase() === "rc") return Boolean(s && s.remote_control);
  return fuzzyMatch(filter, sessionHaystack(s));
}

// contextKnown separates "no reading at all" from "a reading that happens to be
// zero". The daemon keeps them apart on purpose — `contextView` returns a nil
// pointer for the first — and collapsing them here would rebuild the defect #367
// fixed on the server.
//
// Both fields are required and neither is taken as proof of the other: the daemon
// sets them together, and a render path is the wrong place to trust an invariant
// it cannot enforce.
export function contextKnown(s) { return s != null && s.context_tokens != null && s.context_pct != null; }

// contextPct is how full the context window is. The daemon derives it (ADR-0011)
// — window table and rounding included — so this reads a number rather than
// recomputing one. It used to hold a transcription of internal/tui/context.go,
// and with it a rounding that disagreed with Go's on an exact .5.
export function contextPct(s) { return contextKnown(s) ? s.context_pct : 0; }

// contextCell is the CTX column: a dash when unknown, a percentage otherwise —
// including `0%` for a session known to have just been cleared.
export function contextCell(s) { return contextKnown(s) ? `${s.context_pct}%` : "-"; }

// IDLE_PRESETS_MS are the "hide idle after" steps the TUI offers, in its order
// (`idlePresets` in internal/tui/prefs.go). 0 is off, and it is the default.
export const IDLE_PRESETS_MS = [0, 15 * 60000, 30 * 60000, 60 * 60000, 3 * 60 * 60000, 6 * 60 * 60000];

export function idleLabel(ms) {
  if (!ms) return "off (never)";
  const m = ms / 60000;
  return m < 60 ? `${m}m` : `${m / 60}h`;
}

// hiddenByIdle is the twin of the idle arm of `prefs.visible` in
// internal/tui/prefs.go. Three details that are easy to get wrong and all matter:
//
//   - the clock is `last_seen_at`, not the status. The preference is named for
//     idleness but filters on *silence*, so a `working` session whose reports
//     stopped is hidden too — deliberately: it is the same "nothing is happening
//     here" signal;
//   - an unreadable timestamp keeps the session visible. Losing a row over a date
//     that would not parse is worse than showing one row too many;
//   - 0 means never hide.
export function hiddenByIdle(s, afterMs, nowMs) {
  if (!afterMs || afterMs <= 0) return false;
  const t = Date.parse(s && s.last_seen_at);
  if (Number.isNaN(t)) return false;
  return nowMs - t > afterMs;
}

// GROUP_MODES are how the sessions table can be grouped, in the TUI's enum order.
// Kept identical to `groupNames` in internal/tui/sessions.go, and proved so by
// test/fixtures/group-cases.json, which both suites read (#619). A Go test used to
// pull this literal out of the file with a regular expression instead; the case
// list replaced it, because a constant matching says nothing about behaviour at a
// boundary. The names matter as much as the behaviour: an operator's saved
// preference holds the name, so a mode renamed on one client and not the other
// silently resets their grouping — the divergence #544 forbids.
export const GROUP_MODES = ["off", "machine", "project"];

// groupKeyOf is the value a session groups under — the twin of `groupKey` in
// internal/tui/render.go. Both now read the daemon's `project`, the last path
// segment rather than the full directory, so two machines checking out the same
// project land together (#618).
export function groupKeyOf(s, mode) {
  if (!s) return "";
  return mode === "project" ? (s.project || "") : (s.machine || "");
}

// groupSessions turns an ordered list into its groups, preserving the operator's
// sort inside each one.
//
// The TUI sorts by the active key first, then *stably* re-sorts by group key
// (internal/tui/sessionsview.go), so groups come out in key order while the rows
// inside a group keep the chosen sort. JavaScript's sort has been stable since
// ES2019, so the same two-step works here. Doing it in one comparison would lose
// the inner order, which is the mistake this comment exists to prevent.
//
// Each group carries the two figures the TUI's header shows: how many sessions,
// and the sum of their tokens — all four buckets (`totalTokens`), not output
// alone. Both count what is actually on screen, so a filter changes them.
export function groupSessions(list, mode) {
  const rows = list || [];
  if (!mode || mode === "off") return [{ key: null, sessions: rows, count: rows.length, tokens: 0 }];
  const ordered = [...rows].sort((a, b) => {
    const ka = groupKeyOf(a, mode), kb = groupKeyOf(b, mode);
    return ka < kb ? -1 : ka > kb ? 1 : 0;
  });
  const out = [];
  for (const s of ordered) {
    const key = groupKeyOf(s, mode);
    const last = out[out.length - 1];
    if (!last || last.key !== key) out.push({ key, sessions: [s], count: 1, tokens: totalTokens(s.usage || {}) });
    else { last.sessions.push(s); last.count++; last.tokens += totalTokens(s.usage || {}); }
  }
  return out;
}

// The statuses that call for the operator: the session is blocked and needs a
// human. Kept identical to internal/status.Attention — a Go test reads this
// literal and fails on drift, because a dashboard that disagrees with the TUI
// about when to interrupt you is worse than one that never tries (#466).
//
// The dashboard used to decide for itself, and dropped `error`: a session stuck
// on a 529 was drawn like any working one (#538).
// needsAttention covers both reasons to interrupt: a status that means the
// session is blocked, and a call the session raised for itself (ADR-0010). The
// call is not a status — it rides alongside one — so anything deciding whether to
// interrupt has to look at both.
//
// The status half is the daemon's answer since ADR-0011 (#617). The list used to
// be transcribed here, and the dashboard dropped `error` from its copy: a session
// stuck on a 529 was drawn like any working one (#538).
export function needsAttention(session) {
  if (!session) return false;
  return hasCall(session) || Boolean(session.attention);
}

export function attentionCount(sessions) {
  return (sessions || []).filter(needsAttention).length;
}

// REFRESH_MS is how often the dashboard asks for the session list, stream or no
// stream. It matches the TUI's poll (internal/tui/model.go) and exists for the
// same reason: `ended` and `stale` are not stored, they are derived from the
// clock every time the list is read (internal/server/sessions.go). That
// transition changes no field, so it publishes no event, so it reaches a client
// that only listens — never. Relative ages are refreshed by the same tick.
export const REFRESH_MS = 5000;

// SILENCE_MS is how long the dashboard waits to hear anything at all before it
// treats the stream as dead — three missed 10 s keep-alives, the same window the
// TUI applies (internal/tui/sse.go).
//
// It exists because a suspended machine's connection dies without a FIN or an
// RST: the socket stays open as far as the page is concerned, and `read()` blocks
// until the OS gives up on its keepalive probes — minutes. The reconnect path is
// correct and simply never runs, because the loop it guards has not returned
// (#457).
export const SILENCE_MS = 30000;

// streamIsSilent reports whether nothing has been heard since lastHeardAt.
// A stream nothing has ever been heard from is *not* silent: that is a connection
// still being established, and condemning it would abort every connect attempt
// before it opened.
export function streamIsSilent(lastHeardAt, now, limitMs = SILENCE_MS) {
  if (!lastHeardAt) return false;
  return now - lastHeardAt > limitMs;
}

// --- Watcher liveness -------------------------------------------------------
//
// The one rule ADR-0011 leaves deliberately duplicated: a watcher's freshness is
// a function of *now*, so it decays with nothing happening, and a verdict the
// daemon computed would be frozen at the moment it was sent. This endpoint is
// refetched once a minute against a fifteen-second threshold — a dead watcher
// would read as live for up to a minute, on the indicator whose whole job is to
// say the board cannot be trusted (#617, #623).
//
// Duplicated, not unchecked: `test/fixtures/watcher-cases.json` holds the case
// list, and internal/tui proves the same cases. Specified in
// docs/design/watcher-liveness.md § 5 and § 6.

export const WATCHER_STALE_MS = 15000;
export const WATCHER_REPORTING = "reporting";
export const WATCHER_SILENT = "silent";
export const WATCHER_UNREADABLE = "unreadable";

// RFC3339, as Go's time.Parse accepts it. Date.parse alone is far more lenient —
// it takes "2026-08-25" happily, where Go rejects it — and a browser quietly
// accepting what the terminal refuses is the disagreement the shared fixture
// exists to catch.
const RFC3339 = /^\d{4}-\d{2}-\d{2}[Tt]\d{2}:\d{2}:\d{2}(\.\d+)?([Zz]|[+-]\d{2}:\d{2})$/;

// readWatcher turns one recorded heartbeat into a verdict. An unreadable
// timestamp is its own answer, neither health nor silence: the indicator says
// whether the screen can be trusted, and an answer that cannot be read answers
// that with no — but the fault is in what vigie recorded, not on that machine.
export function readWatcher(seen, nowMs) {
  if (!seen) return WATCHER_SILENT;
  if (!RFC3339.test(seen)) return WATCHER_UNREADABLE;
  const t = Date.parse(seen);
  if (Number.isNaN(t)) return WATCHER_UNREADABLE;
  return nowMs - t > WATCHER_STALE_MS ? WATCHER_SILENT : WATCHER_REPORTING;
}

export function watcherAlarm(verdict) { return verdict !== WATCHER_REPORTING; }

// fleetWatchers counts the machines known and names those that beat and then
// stopped, split by cause. A machine with no recorded heartbeat is reporting
// through hooks alone — a deployment choice, not a fault — so it is counted and
// never listed.
//
// Names are sorted so the alarm text is stable. Go sorts by bytes and JavaScript
// by UTF-16 code units; the two part company only outside ASCII, which a hostname
// is not.
export function fleetWatchers(machines, nowMs) {
  const silent = [], unreadable = [];
  const entries = Object.entries(machines || {});
  for (const [name, seen] of entries) {
    if (!seen) continue; // never beat: hooks-only, by choice
    const v = readWatcher(seen, nowMs);
    if (v === WATCHER_SILENT) silent.push(name);
    else if (v === WATCHER_UNREADABLE) unreadable.push(name);
  }
  return { known: entries.length, silent: silent.sort(), unreadable: unreadable.sort() };
}

// fleetAlarm reports whether the statuses on screen may be frozen anywhere in the
// fleet. It is true with no names in one case: nothing beating at all — no
// watcher was ever started — where no single machine qualifies as having stopped
// yet nothing is refreshing anything.
export function fleetAlarm(machines, nowMs) {
  const { known, silent, unreadable } = fleetWatchers(machines, nowMs);
  if (silent.length || unreadable.length) return { alarm: true, known, silent, unreadable };
  for (const seen of Object.values(machines || {})) {
    if (readWatcher(seen, nowMs) === WATCHER_REPORTING) {
      return { alarm: false, known, silent: [], unreadable: [] };
    }
  }
  return { alarm: true, known, silent: [], unreadable: [] };
}

// fleetAlarmDetail is the indicator's text: how much of the fleet is affected and
// which machines, so the operator does not have to open another tab to learn
// where to go.
export function fleetAlarmDetail(known, silent, unreadable) {
  const names = [...silent, ...unreadable].sort();
  if (!names.length) return "not reporting · statuses may be frozen";
  const what = silent.length === 0 ? "unreadable heartbeat" : "not reporting";
  return `${names.length} of ${known} ${what} (${names.join(", ")})`;
}

// watcherCell is a machine's own verdict as the Machines card shows it: the word
// and the class. "time?" is a heartbeat vigie recorded and cannot read — a fault
// on this side, not on that machine — and saying which keeps the alarm from
// sending the operator to the wrong host (watcher-liveness.md § 5).
//
// It is here rather than inline in the card so it can be proved: the card itself
// is built inside app.js, which the live harness cannot reach.
export function watcherCell(verdict) {
  if (verdict === WATCHER_UNREADABLE) return { cls: "w-bad", text: "time?" };
  return watcherAlarm(verdict) ? { cls: "w-bad", text: "none" } : { cls: "w-ok", text: "live" };
}
