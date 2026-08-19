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

// apiErrorLabel names the common Claude API error codes. The TUI has the same
// list (internal/tui/render.go) and the two must agree: they are one vocabulary
// with two implementations, and architecture.md binds the dashboard to the TUI
// on content. test/fixtures/api-error-labels.json is that agreement, read by
// both test suites (#584).
export function apiErrorLabel(code) {
  switch (Number(code)) {
    case 429: return "429 Rate limited";
    case 500: return "500 Internal server error";
    case 529: return "529 Overloaded";
    default: return String(code);
  }
}

// detailText is the Detail cell's content. A raised call takes it, because it is
// the reason the row is pulsing and it outranks everything else. An API error
// comes next: once the API answers 529 the tool that ran last is of no interest,
// and the code is what separates an outage from throttling (#584).
export function detailText(s) {
  if (hasCall(s)) return s.call_message ? esc(s.call_message) : "called you";
  if (s && s.status === "error" && s.api_error_status) return esc(apiErrorLabel(s.api_error_status));
  return s.detail ? esc(s.detail) : "-";
}

export function humanTokens(n) { n = Number(n) || 0; if (n >= 1e6) return trim(n / 1e6) + "M"; if (n >= 1e3) return trim(n / 1e3) + "k"; return String(n); }
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

export const shortModel = (m) => (m || "").replace(/^claude-/, "");
export function projectName(dir) { if (!dir) return "-"; const p = dir.replace(/\/+$/, "").split("/"); return p[p.length - 1] || dir; }
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
export const RANK_ORDER = ["stalled", "working", "thinking", "compacting", "waiting", "idle", "error", "stale", "ended"];

// rank places an unknown status last, never first: a status this build has never
// heard of is the one we can say least about.
export function rank(status) {
  const i = RANK_ORDER.indexOf(status);
  return i < 0 ? RANK_ORDER.length : i;
}

// shortId is the first eight characters of a session id, the fallback the table
// shows for a session Claude has not titled yet. Kept at eight to match
// `shortID` in internal/tui/render.go: the two clients must name a row the same
// way, and the filter below searches that name (#545).
export function shortId(id) {
  const r = [...String(id == null ? "" : id)];
  return r.length > 8 ? r.slice(0, 8).join("") : r.join("");
}

// sessionName is the conversation title, falling back to the short id — the twin
// of `sessionName` in internal/tui/render.go.
export function sessionName(s) {
  if (!s) return "";
  return s.title ? s.title : shortId(s.id);
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
  return [sessionName(s), f(s.machine), projectName(s.project_dir), f(s.git_branch), f(s.status)].join(" ");
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

// modelVersion splits a short model name ("opus-4-8") into family and versions —
// the twin of internal/tui/context.go. The strict numeric test matters: Go's
// strconv.Atoi rejects "4x" and yields 0, where parseInt would happily return 4
// and silently move a model into the wrong context window.
export function modelVersion(short) {
  const parts = String(short == null ? "" : short).split("-");
  const num = (p) => (/^[+-]?\d+$/.test(p == null ? "" : p) ? Number(p) : 0);
  return { family: parts[0] || "", major: num(parts[1]), minor: num(parts[2]) };
}

// contextWindow is how many tokens the model's context holds. Kept identical to
// internal/tui/context.go; a shared fixture checks both against the same cases.
export function contextWindow(model) {
  const BIG = 1000000, BASE = 200000;
  const { family, major, minor } = modelVersion(shortModel(model));
  if (family === "fable") return BIG;
  if (family === "opus" || family === "sonnet") return (major > 4 || (major === 4 && minor >= 6)) ? BIG : BASE;
  return BASE;
}

// contextKnown separates "no reading at all" from "a reading that happens to be
// zero". The daemon keeps them apart on purpose — `contextView` returns a nil
// pointer for the first — and collapsing them here would rebuild the defect #367
// fixed on the server.
export function contextKnown(s) { return s != null && s.context_tokens != null; }

export function contextPct(s) {
  if (!contextKnown(s) || s.context_tokens <= 0) return 0;
  return (s.context_tokens / contextWindow(s.model)) * 100;
}

// contextCell is the CTX column: a dash when unknown, a percentage otherwise —
// including `0%` for a session known to have just been cleared.
//
// Go formats with %.0f, which rounds half to even, while Math.round rounds half
// up. They part company only on an exact .5, which a token count over a window of
// 200 000 or 1 000 000 does not produce in practice; the shared fixture stays off
// that boundary rather than pretending it does not exist.
export function contextCell(s) { return contextKnown(s) ? `${Math.round(contextPct(s))}%` : "-"; }

// PERMISSION_MODES is the #303 taxonomy, raw value to label. An unrecognised
// non-empty value is shown as it came rather than relabelled: a new mode must
// never read as the safe default (#304).
export const PERMISSION_MODES = {
  "": "-", default: "manual", acceptEdits: "accept",
  plan: "plan", auto: "auto", bypassPermissions: "bypass",
};

export function modeLabel(raw) {
  const r = raw == null ? "" : String(raw);
  return Object.prototype.hasOwnProperty.call(PERMISSION_MODES, r) ? PERMISSION_MODES[r] : r;
}

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
// Kept identical to `groupNames` in internal/tui/model.go — a Go test reads this
// literal and fails on drift, because a grouping an operator finds in one window
// and not the other is the divergence #544 forbids.
export const GROUP_MODES = ["off", "machine", "project"];

// groupKeyOf is the value a session groups under — the twin of `groupKey` in
// internal/tui/render.go. Project groups by the last path segment, not the full
// directory, so two machines checking out the same project land together.
export function groupKeyOf(s, mode) {
  if (!s) return "";
  return mode === "project" ? projectName(s.project_dir) : (s.machine || "");
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
export const ATTENTION = ["waiting", "error", "stalled"];

// needsAttention covers both reasons to interrupt: a status that means the
// session is blocked, and a call the session raised for itself (ADR-0010). The
// call is not a status — it rides alongside one — so anything deciding whether to
// interrupt has to look at both.
export function needsAttention(session) {
  if (!session) return false;
  return hasCall(session) || ATTENTION.includes(session.status);
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
