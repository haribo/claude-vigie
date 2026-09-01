// claude-vigie — read-only web dashboard, a browser mirror of the terminal UI.
// It only reads the API (observe-only). The operator's token lives in localStorage
// and is sent as a bearer token; all data is HTML-escaped before it reaches the
// DOM, since a session's title/branch/activity are transcript-derived and the
// token makes DOM-based XSS a real risk (issue #161). Dynamic sizes are applied
// via CSSOM (element.style), never inline attributes, to keep the strict CSP.

import {
  esc, dash, trim, hasCall, detailText, humanTokens, ageSec, relAge, relResetHint,
  totalTokens, sparkSVG, migrateKeys, fullColOrder, colHidden, rank,
  adoptLegacyKey, needsAttention, attentionCount, streamIsSilent, REFRESH_MS,
  readWatcher, fleetAlarm, fleetAlarmDetail, watcherCell,
  matchesFilter, GROUP_MODES, groupSessions, contextKnown, contextPct, contextCell, migrateV1Columns,
  IDLE_PRESETS_MS, idleLabel, hiddenByIdle, STATUSES, SORT_COMPARATORS, DEFAULT_SORT,
  boardState, emptyMessage, attentionIds, enteredAttention, nextAttention,
} from "./lib.js";

// Both keys were named for the old brand. They hold live state — a signed-in
// token and a column layout — so the old name is read once and carried over
// rather than dropped (adoptLegacyKey, #478).
const TOKEN_KEY = "vigie_token";

let token = adoptLegacyKey(localStorage, "cf_token", TOKEN_KEY) || "";
let sessions = [], byId = new Map();
let usage = null, platform = null, stats = null, settings = null, ver = null, watcher = null;
let activeTab = "sessions", detailId = null;
// Whether ended sessions are listed. A Settings preference rather than a control
// on the table, as in the TUI: `hidden N` is shown only when something is hidden
// (sessions-chrome.md § 2), so a button living there would vanish the moment it
// had been used and leave no way back (#548).
const ENDED_KEY = "vigie_show_ended";
const NOTIFY_KEY = "vigie_notify";
let showEnded = localStorage.getItem(ENDED_KEY) === "1";
// The active filter. Held here rather than read from the DOM so a repaint of the
// table can never change it (#545).
let filter = "";
// How the table is grouped. Browser-local like the column layout and the sort:
// it is a view preference, and vigie stores nothing about how an operator works
// (ADR-0007). Read once here; an unknown stored value degrades to "off" (#546).
const GROUP_KEY = "vigie_group";
// How long a session may go unheard before it leaves the table. Browser-local
// like the rest of the view preferences (#547).
const IDLE_KEY = "vigie_idle_hide";
let idleHideAfter = IDLE_PRESETS_MS.includes(Number(localStorage.getItem(IDLE_KEY))) ? Number(localStorage.getItem(IDLE_KEY)) : 0;
let groupBy = GROUP_MODES.includes(localStorage.getItem(GROUP_KEY) || "") ? localStorage.getItem(GROUP_KEY) : "off";
let sortKey = DEFAULT_SORT.key, sortDir = DEFAULT_SORT.dir;   // 1 = the comparator's own order
let statsPeriod = "7d", statsLoaded = false, settingsLoaded = false;
let liveCtrl = null, liveRetry = null, tickTimer = null, metaTimer = null;
// When the stream last delivered any bytes, keep-alive comments included. It is
// what the silence watchdog measures; 0 means nothing has been heard yet.
let lastHeardAt = 0;

const $ = (id) => document.getElementById(id);

// paint writes html into an element only when it differs from what is already
// there, and keeps the scroll position when it does write.
//
// The refresh tick redraws every 5 s whether or not anything changed (#538).
// Rewriting innerHTML unconditionally throws away everything the DOM was holding:
// the operator's text selection, keyboard focus on a row, and — because
// #tab-sessions contains the scroll container — the scroll position, which would
// send a long fleet list back to the top twelve times a minute. Identical HTML
// means nothing on screen would differ, so the cheapest correct redraw is none.
const painted = new Map();
function paint(id, html) {
  if (painted.get(id) === html) return false;
  painted.set(id, html);
  const el = $(id);
  // Read before the write: replacing innerHTML destroys the old scroll container.
  const scroller = el.querySelector(".table-scroll");
  const top = scroller ? scroller.scrollTop : 0;
  el.innerHTML = html;
  if (top) {
    const fresh = el.querySelector(".table-scroll");
    if (fresh) fresh.scrollTop = top;
  }
  return true;
}
// ---------- API ----------
function authHeaders() { return { "Authorization": "Bearer " + token }; }
async function api(path) {
  const res = await fetch(path, { headers: authHeaders() });
  if (res.status === 401) { onUnauthorized(); throw new Error("unauthorized"); }
  if (!res.ok) throw new Error(path + " → " + res.status);
  return res.json();
}

// ---------- tabs ----------
const TABS = [{ id: "sessions", label: "Sessions" }, { id: "stats", label: "Stats" }, { id: "machines", label: "Machines" }, { id: "settings", label: "Settings" }];
function renderTabs() {
  // Every reason to interrupt, not just `waiting`: the badge is the one number an
  // operator on another tab sees, and it counted a third of them (#538).
  const attention = attentionCount(sessions);
  const html = TABS.map((t) => {
    const badge = (t.id === "sessions" && attention) ? `<span class="badge">${attention}</span>` : "";
    return `<button class="tab ${t.id === activeTab ? "active" : ""}" data-tab="${t.id}">${t.label}${badge}</button>`;
  }).join("");
  if (!paint("tabbar", html)) return; // the tick calls this every 5 s; listeners survive
  $("tabbar").querySelectorAll(".tab").forEach((b) => b.addEventListener("click", () => switchTab(b.dataset.tab)));
}
// syncFilterBar keeps the bar on the Sessions tab only, and its count in step
// with the table. The bar lives outside the painted region, so nothing else
// updates it.
function renderGroupSelect() {
  const el = $("group-select");
  el.innerHTML = GROUP_MODES.map((m) => `<option value="${m}"${m === groupBy ? " selected" : ""}>${m}</option>`).join("");
}

function syncFilterBar() {
  const show = activeTab === "sessions" && !detailId && $("gate").hidden;
  $("filterbar").hidden = !show;
  if (!show) return;
  $("filter-count").textContent = filter ? `${visibleSessions().length} of ${preferenceVisible().length}` : "";
}

function switchTab(id) {
  activeTab = id; detailId = null; $("view-detail").hidden = true;
  ["sessions", "stats", "machines", "settings"].forEach((t) => $("tab-" + t).hidden = (t !== id));
  if (id === "machines") renderMachines();
  if (id === "stats") { if (!statsLoaded) loadStats(); else renderStats(); }
  if (id === "settings") { if (!settingsLoaded) loadSettings(); else renderSettings(); }
  renderTabs(); syncFilterBar(); if (usage || platform) renderBottom(); window.scrollTo(0, 0);
}

// ---------- Sessions ----------
// Each column carries how to render its cell, so the header and the rows are both
// driven by the (operator-ordered, filtered) column list — see activeCols (#309).
const COLS = [
  // The TUI's set, in its built-in order (internal/tui/render.go `columns`), plus
  // one affordance a pointer needs and a keyboard does not. A Go test compares the
  // two key sets and fails on drift (#550, #544).
  //
  // s.name, not `title || id`: the daemon names the session (#618), and an untitled one by the first
  // eight characters of its id in the TUI, and the filter searches that name — a
  // column showing 36 characters of a key the filter cannot reach is a trap (#545).
  { key: "name", label: "Session", cmp: SORT_COMPARATORS.name,
    cell: (s) => { const n = esc(s.name || ""); return `<td class="name" title="${esc(s.title || s.id)}"><span class="nm">${n}</span></td>`; } },
  { key: "user", label: "User", cmp: SORT_COMPARATORS.user,
    cell: (s) => `<td class="${s.user ? "dim" : "faint"}">${esc(dash(s.user))}</td>` },
  { key: "machine", label: "Machine", cmp: SORT_COMPARATORS.machine,
    cell: (s) => `<td class="dim">${esc(s.machine)}</td>` },
  { key: "dir", label: "Dir", cmp: SORT_COMPARATORS.dir,
    cell: (s) => { const p = esc(s.project || ""); return `<td class="proj" title="${esc(s.project_dir || "")}">${p}</td>`; } },
  { key: "branch", label: "Branch", cmp: SORT_COMPARATORS.branch,
    cell: (s) => { const b = esc(dash(s.git_branch)); return `<td class="branch dim" title="${b}">${b}</td>`; } },
  { key: "model", label: "Model", cmp: SORT_COMPARATORS.model,
    cell: (s) => `<td class="${s.model ? "dim" : "faint"}">${esc(dash(s.model_short))}</td>` },
  { key: "effort", label: "Effort", cmp: SORT_COMPARATORS.effort,
    cell: (s) => `<td class="${s.effort ? "dim" : "faint"}">${esc(dash(s.effort))}</td>` },
  // Unknown and known-to-be-zero are different states on screen, and the daemon
  // keeps them apart precisely so this cell can say so (#367).
  { key: "ctx", label: "Ctx", num: true, cmp: SORT_COMPARATORS.ctx,
    cell: (s) => { const k = contextKnown(s); const p = contextPct(s); const cls = !k ? "faint" : p >= 85 ? "ctx-hot" : p >= 60 ? "ctx-warn" : ""; return `<td class="num ${cls}">${contextCell(s)}</td>`; } },
  { key: "out", label: "Out", num: true, cmp: SORT_COMPARATORS.out,
    cell: (s) => `<td class="num">${humanTokens((s.usage || {}).output_tokens)}</td>` },
  { key: "total", label: "Total", num: true, cmp: SORT_COMPARATORS.total,
    cell: (s) => `<td class="num">${humanTokens(totalTokens(s.usage || {}))}</td>` },
  { key: "seen", label: "Seen", num: true, cmp: SORT_COMPARATORS.seen,
    cell: (s) => `<td class="num dim">${relAge(s.last_seen_at)}</td>` },
  { key: "act", label: "Act", nosort: true,
    cell: (s) => `<td>${sparkSVG(s.samples)}</td>` },
  { key: "rc", label: "RC", cmp: SORT_COMPARATORS.rc,
    cell: (s) => `<td>${s.remote_control ? '<span class="rc-on" title="Remote control on">◉</span>' : '<span class="rc-off" title="Remote control off">○</span>'}</td>` },
  { key: "status", label: "Status", cmp: SORT_COMPARATORS.status,
    cell: (s) => { const st = STATUSES.includes(s.status) ? s.status : "idle"; return `<td><span class="pill st-${st}${hasCall(s) ? " call" : ""}"><span class="dot"></span>${st}</span></td>`; } },
  // An unrecognised mode is surfaced raw, never relabelled "manual": a new mode
  // must not read as the safe default (#304).
  { key: "mode", label: "Mode", cmp: SORT_COMPARATORS.mode,
    cell: (s) => `<td class="${s.permission_mode ? "mode-" + esc(s.mode_label) : "faint"}">${esc(s.mode_label || "")}</td>` },
  { key: "detail", label: "Detail", nosort: true,
    cell: (s) => { const d = detailText(s); const cls = hasCall(s) ? "detail call" : (s.status === "waiting" ? "detail wait" : "detail"); return `<td class="${cls}" title="${d}">${d}</td>`; } },
  // A gesture, not content: a keyboard opens the detail with Enter on the row, a
  // pointer needs something to click. Declared as such in the Go guard (#544).
  { key: "open", label: "", nosort: true,
    cell: () => `<td><button class="det-btn" aria-label="Open detail" title="Open detail"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M9 6l6 6-6 6"/></svg></button></td>` },
];


// COLS_KEY holds the operator's column layout for this browser, {order, hidden}:
// the display order of ALL columns plus the hidden set. Client-local like the
// token. Hiding a column keeps its position — only its visibility changes (#315).
// v2 because #550 renamed four keys at once. A layout is live state — renaming
// in place would drop a column out of the operator's order, and if it had been
// hidden it would silently reappear (#393, #478). The carry-over runs exactly
// once: v1 is read, remapped, written under v2, and removed. It cannot run twice,
// which matters because the remap is not idempotent — `activity` becomes `act`
// and `act` becomes `open`, so a second pass would turn the sparkline into the
// detail button.
const COLS_KEY = "vigie_columns_v2";
const COLS_KEY_V1 = "vigie_columns";
const MANDATORY_COLS = new Set(["name", "status"]);
const COL_KEYS = COLS.map((c) => c.key);


function loadLayout() {
  try {
    const v2 = localStorage.getItem(COLS_KEY);
    if (v2 !== null) {
      const v = JSON.parse(v2);
      if (v && Array.isArray(v.order)) return { order: migrateKeys(v.order), hidden: migrateKeys(Array.isArray(v.hidden) ? v.hidden : []) };
    }
    // One-time carry-over: the pre-#550 key set, and before it the old brand's.
    const raw = adoptLegacyKey(localStorage, "cf_columns", COLS_KEY_V1);
    if (raw !== null) {
      const v = JSON.parse(raw || "null");
      let l = { order: [], hidden: [] };
      if (Array.isArray(v)) l = { order: v.slice(), hidden: [] }; // the oldest, visible-only form
      else if (v && Array.isArray(v.order)) l = { order: v.order, hidden: Array.isArray(v.hidden) ? v.hidden : [] };
      l = { order: migrateV1Columns(l.order), hidden: migrateV1Columns(l.hidden) };
      saveLayout(l);
      localStorage.removeItem(COLS_KEY_V1);
      return l;
    }
  } catch (e) { /* fall through to default */ }
  return { order: [], hidden: [] };
}
function saveLayout(l) { localStorage.setItem(COLS_KEY, JSON.stringify(l)); }


// activeCols is the visible columns in display order — the base for the table.
function activeCols() {
  const l = loadLayout(), byKey = new Map(COLS.map((c) => [c.key, c]));
  return fullColOrder(l.order, COL_KEYS).filter((k) => !colHidden(l.hidden, k, MANDATORY_COLS)).map((k) => byKey.get(k));
}

// pickerCols is every column in display order (stable — hiding never moves one).
function pickerCols() {
  const l = loadLayout(), byKey = new Map(COLS.map((c) => [c.key, c]));
  return fullColOrder(l.order, COL_KEYS).map((k) => byKey.get(k));
}
function colVisible(key) { return !colHidden(loadLayout().hidden, key, MANDATORY_COLS); }

function toggleCol(key) {
  if (MANDATORY_COLS.has(key)) return;
  const l = loadLayout(), i = l.hidden.indexOf(key);
  if (i >= 0) l.hidden.splice(i, 1); else l.hidden.push(key);
  saveLayout(l);
}
function moveCol(key, dir) {
  const l = loadLayout(); l.order = fullColOrder(l.order, COL_KEYS);
  const i = l.order.indexOf(key), j = i + dir;
  if (i < 0 || j < 0 || j >= l.order.length) return;
  [l.order[i], l.order[j]] = [l.order[j], l.order[i]];
  saveLayout(l);
}
// notHidden applies the two visibility *preferences* — the twin of
// `prefs.visible` in internal/tui/prefs.go. The text filter is separate: it has
// its own count, and the TUI reports it on its own line too.
//
// One predicate, used by the table and by the hidden count both, so the screen
// can never claim a smaller fleet than it is filtering (sessions-chrome.md § 2,
// test 3).
function notHidden(s) {
  if (!showEnded && s.status === "ended") return false;
  return !hiddenByIdle(s, idleHideAfter, Date.now());
}
function preferenceVisible() { return sessions.filter(notHidden); }

function visibleSessions() {
  // Same order as the TUI (internal/tui/sessions.go): the visibility
  // preferences first, then the filter, then the sort.
  const list = preferenceVisible().filter((s) => matchesFilter(s, filter));
  const col = COLS.find((c) => c.key === sortKey);
  return col && col.cmp ? [...list].sort((a, b) => col.cmp(a, b) * sortDir) : list;
}

function renderSessions() {
  const cols = activeCols();
  const heads = cols.map((c) => {
    const sorted = c.key === sortKey;
    const arrow = sorted ? `<span class="arrow">${sortDir === 1 ? "▼" : "▲"}</span>` : "";
    const cls = [c.num ? "num" : "", c.nosort ? "nosort" : "", sorted ? "sorted" : ""].filter(Boolean).join(" ");
    return `<th class="${cls}" ${c.nosort ? "" : `data-sort="${c.key}"`}>${c.label}${arrow}</th>`;
  }).join("");
  const row = (s) => {
    const st = STATUSES.includes(s.status) ? s.status : "idle";
    // A call reuses the attention mechanism — the left border in --st — and adds
    // a faint tint of the same colour. No new colour anywhere (ADR-0010).
    // The set is consulted, never restated: it is shared with the TUI and the
    // GNOME indicator, and restating it here is how `error` went unmarked (#538).
    const attn = needsAttention(s) ? " attn" : "";
    const call = hasCall(s) ? " call" : "";
    const cells = cols.map((c) => c.cell(s)).join("");
    return `<tr class="st-${st}${attn}${call}" data-id="${esc(s.id)}" tabindex="0">${cells}</tr>`;
  };
  // One table, group headers as full-width rows: a separate table per group would
  // give each its own column widths and the eye could no longer read down a
  // column. The header carries what the TUI's does — the key, how many sessions,
  // and their combined tokens (all four buckets, not output alone) (#546).
  const rows = groupSessions(visibleSessions(), groupBy).map((g) =>
    (g.key === null ? "" : `<tr class="grouphead"><td colspan="${cols.length}">▸ <span class="gk">${esc(dash(g.key))}</span> <span class="gm">(${g.count} · ${humanTokens(g.tokens)})</span></td></tr>`)
    + g.sessions.map(row).join("")).join("");
  // The TUI says which of the two silences this is, and so must this: an empty
  // fleet and a filter that matched nothing look identical otherwise.
  const msg = emptyMessage(visibleSessions().length, Boolean(filter), refreshFailed, everLoaded);
  const empty = msg ? `<div class="empty">${esc(msg)}</div>` : "";
  const html = `<div class="table-wrap"><div class="table-scroll"><table><thead><tr>${heads}</tr></thead><tbody>${rows}</tbody></table></div>${empty}</div>`;
  if (!paint("tab-sessions", html)) return; // nothing on screen would change; listeners are still bound
  $("tab-sessions").querySelectorAll("th[data-sort]").forEach((th) => th.addEventListener("click", () => {
    const k = th.dataset.sort; if (k === sortKey) sortDir = -sortDir; else { sortKey = k; sortDir = 1; } renderSessions();
  }));
  syncFilterBar();
  if (usage || platform) renderBottom();
  $("tab-sessions").querySelectorAll("tbody tr").forEach((tr) => {
    const go = () => openDetail(tr.dataset.id);
    tr.addEventListener("click", go);
    tr.addEventListener("keydown", (e) => { if (e.key === "Enter") go(); });
  });
}

// ---------- Machines ----------
function renderMachines() {
  const m = {};
  sessions.forEach((s) => {
    const a = m[s.machine] || (m[s.machine] = { name: s.machine, user: s.user || "-", n: 0, out: 0, seen: s.last_seen_at });
    a.n++; a.out += (s.usage ? s.usage.output_tokens || 0 : 0); a[s.status] = (a[s.status] || 0) + 1;
    if (ageSec(s.last_seen_at) < ageSec(a.seen)) a.seen = s.last_seen_at;
  });
  const cards = Object.values(m).sort((a, b) => b.n - a.n).map((a) => {
    const brk = STATUSES.filter((k) => a[k]).map((k) => `<span class="b st-${k}"><span class="dot"></span>${a[k]} <small>${k}</small></span>`).join("");
    const wv = watcher && watcher.versions && watcher.versions[a.name];
    const wver = wv && wv.version ? wv.version : "—";
    // Each machine's own verdict, beside the version it already showed. "⚠ time?"
    // is a heartbeat vigie recorded and cannot read — a fault on this side, not on
    // that machine — and saying which keeps the alarm from sending the operator to
    // the wrong host (watcher-liveness.md § 5).
    const seen = (watcher && watcher.machines) ? watcher.machines[a.name] : undefined;
    const { cls: wcls, text: wtxt } = watcherCell(readWatcher(seen === undefined ? "" : seen, Date.now()));
    return `<div class="mach">
      <div class="mach-head"><span class="nm">${esc(a.name)}</span><span class="u">${esc(a.user)}</span></div>
      <div class="mach-row"><span class="big">${a.n}<small>${a.n === 1 ? "session" : "sessions"}</small></span></div>
      <div class="mach-brk">${brk}</div>
      <div class="mach-foot"><span>output <b>${humanTokens(a.out)}</b></span><span>watcher <b class="${wcls}">${wtxt}</b> <small>${esc(wver)}</small></span><span>last seen <b>${relAge(a.seen)}</b></span></div>
    </div>`;
  }).join("");
  $("tab-machines").innerHTML = sessions.length ? `<div class="mach-grid">${cards}</div>` : '<div class="empty">No machines reporting.</div>';
}

// ---------- Stats ----------
async function loadStats() {
  try { stats = await api("/api/stats"); statsLoaded = true; renderStats(); } catch (e) { /* stats optional */ }
}
// The dashboard sums a rolling window; the terminal buckets history. Both offered
// a button called `Week`, and it meant the last seven days as one figure here and
// twelve ISO weeks stacked by model there — same word, same tab, two numbers, and
// neither window said which one you were reading (#666).
//
// The labels now name the window they are, so the two stop contradicting each
// other on a word. They converge properly when the dashboard grows the terminal's
// bucketed chart, which is the browser's own piece of work (#667), not a rename.
//
// The field is `id` rather than the obvious `key`, on purpose:
// `TestDashboardSharesTheColumnSet` scrapes this file for objects keyed that way
// and reads every one it finds as a table column, so a second array of the same
// shape reports as a dashboard column the TUI lacks. Naming the field differently
// keeps the two apart — and this comment avoids spelling the pattern out, having
// been caught by it once.
const PERIODS = [
  { id: "24h", days: 1, phrase: "the last 24 hours" },
  { id: "7d", days: 7, phrase: "the last 7 days" },
  { id: "30d", days: 30, phrase: "the last 30 days" },
  { id: "1y", days: 365, phrase: "the last year" },
  { id: "all", days: Infinity, phrase: "all time" },
];
const periodOf = (id) => PERIODS.find((p) => p.id === id) || PERIODS[1];
function renderStats() {
  // Not just "no stats yet": a machine covered only by the watcher accrues tokens
  // and never a second of time, because only hooks close a status interval
  // (docs/design/status-time.md § 2). Waiting and installing the hooks are
  // different actions, and the panel used to describe the second as the first
  // (#668).
  if (!stats) { $("tab-stats").innerHTML = '<div class="muted-note">No stats yet — tokens accrue from any report; time needs the reporting hooks (<code>vigie hooks install</code>).</div>'; return; }
  const cutoff = Date.now() - periodOf(statsPeriod).days * 86400000;
  const daily = (stats.daily || []).filter((d) => { const t = Date.parse(d.day + "T00:00:00Z"); return Number.isNaN(t) || t >= cutoff; });
  const out = daily.reduce((n, d) => n + (d.output_tokens || 0), 0);
  const work = daily.reduce((n, d) => n + (d.working_seconds || 0), 0);
  const wait = daily.reduce((n, d) => n + (d.waiting_seconds || 0), 0);
  const idle = daily.reduce((n, d) => n + (d.idle_seconds || 0), 0);
  const active = work + wait + idle;
  const machines = new Set(sessions.map((s) => s.machine)).size;
  const periods = PERIODS.map((p) => `<button class="period ${p.id === statsPeriod ? "active" : ""}" data-p="${p.id}">${p.id}</button>`).join("");
  const top = (stats.top_sessions || []).slice(0, 5).map((s, i) =>
    `<div class="top"><span class="rk">${i + 1}</span><span class="nm">${esc(s.name)}</span><span class="mc">${esc(s.machine)}</span><span class="tk">${humanTokens(s.output_tokens)}</span></div>`).join("")
    || '<div class="muted-note">No sessions ranked yet.</div>';
  const pct = (v) => active > 0 ? (v / active * 100) : 0;
  $("tab-stats").innerHTML = `
    <div class="stat-periods">${periods}</div>
    <div class="stat-grid">
      <div class="kpi"><div class="k">Output tokens</div><div class="v">${humanTokens(out)}</div></div>
      <div class="kpi"><div class="k">Active time</div><div class="v">${trim(active / 3600)}<small> h</small></div></div>
      <div class="kpi"><div class="k">Sessions</div><div class="v">${stats.session_count || sessions.length}</div></div>
      <div class="kpi"><div class="k">Machines</div><div class="v">${machines}</div></div>
    </div>
    <div class="stat-cols">
      <div class="card"><h3>Top sessions — output</h3>${top}</div>
      <div class="card"><h3>Time by status (${periodOf(statsPeriod).phrase})</h3>
        <div class="timebar"><i class="st-working" data-w="${pct(work)}"></i><i class="st-waiting" data-w="${pct(wait)}"></i><i class="st-idle" data-w="${pct(idle)}"></i></div>
        <div class="timeleg">
          <span class="b st-working"><span class="dot"></span><b>${trim(work / 3600)}h</b> working</span>
          <span class="b st-waiting"><span class="dot"></span><b>${trim(wait / 3600)}h</b> waiting</span>
          <span class="b st-idle"><span class="dot"></span><b>${trim(idle / 3600)}h</b> idle</span>
        </div>
      </div>
    </div>`;
  $("tab-stats").querySelectorAll(".timebar i[data-w]").forEach((i) => { i.style.width = i.dataset.w + "%"; });
  $("tab-stats").querySelectorAll(".period").forEach((b) => b.addEventListener("click", () => { statsPeriod = b.dataset.p; renderStats(); }));
}

// ---------- Settings (read-only) ----------
async function loadSettings() {
  try { settings = await api("/api/settings"); } catch (e) { settings = {}; }
  settingsLoaded = true; renderSettings();
}
function platformClass(p) {
  if (!p || !p.indicator || p.indicator === "none") return ["ok", "operational"];
  return { minor: ["warn", "degraded"], major: ["bad", "major outage"], critical: ["bad", "critical outage"] }[p.indicator] || ["warn", p.indicator];
}
// notifyRowHTML is the Settings control, and it says why it cannot work when it
// cannot — a toggle that silently does nothing is worse than no toggle (#667).
function notifyRowHTML() {
  if (!notifySupported()) return `<span class="muted-note">${esc(notifyBlockedReason())}</span>`;
  if (Notification.permission === "denied") {
    return '<span class="muted-note">blocked for this site — allow notifications in the browser to enable</span>';
  }
  return `<label class="col-tog"><input type="checkbox" id="notify-toggle"${notifyOn ? " checked" : ""}> notify</label>`;
}

function renderSettings() {
  const retention = settings && settings.session_retention ? settings.session_retention : "kept forever";
  const [pcls, ptxt] = platformClass(platform);
  const colRows = pickerCols().map((c) => {
    const on = colVisible(c.key), req = MANDATORY_COLS.has(c.key), label = esc(c.label || c.key);
    return `<div class="col-row">
      <label class="col-tog"><input type="checkbox" data-col="${c.key}" ${on ? "checked" : ""} ${req ? "disabled" : ""}> ${label}${req ? " <small>required</small>" : ""}</label>
      <span class="col-move"><button data-mv-up="${c.key}" title="Move up" aria-label="Move up">↑</button><button data-mv-down="${c.key}" title="Move down" aria-label="Move down">↓</button></span>
    </div>`;
  }).join("");
  $("tab-settings").innerHTML = `
    <div class="settings">
      <div class="set-note"><span>ℹ</span><span><b>Server</b> settings are read-only here — claude-vigie is observe-only; change them on the daemon. Your <b>column layout</b> and sort are saved in this browser.</span></div>
      <div class="set-row"><span class="k">Server<small>the daemon this dashboard is served by</small></span><span class="v">${esc(location.origin)}</span></div>
      <div class="set-row"><span class="k">Session retention<small>how long closed sessions are kept</small></span><span class="v">${esc(retention)}</span></div>
      <div class="set-row"><span class="k">Platform status<small>polled from status.claude.com</small></span><span class="v ${pcls === "ok" ? "ok" : ""}">● ${esc(ptxt)}</span></div>
      <div class="set-row"><span class="k">Token<small>stored in this browser, sent as a bearer token</small></span><span class="v">connected <button class="signout2" id="signout2">sign out</button></span></div>
      <div class="set-row"><span class="k">Desktop notifications<small>when a session starts calling you — asked once, saved in this browser</small></span><span class="v">${notifyRowHTML()}</span></div>
      <div class="set-row"><span class="k">Show ended sessions<small>keep closed sessions in the table — saved in this browser</small></span><span class="v"><label class="col-tog"><input type="checkbox" id="ended-toggle"${showEnded ? " checked" : ""}> show</label></span></div>
      <div class="set-row"><span class="k">Hide idle after<small>a session unheard from for longer leaves the table — saved in this browser</small></span><span class="v"><select id="idle-select" aria-label="Hide idle after">${IDLE_PRESETS_MS.map((ms) => `<option value="${ms}"${ms === idleHideAfter ? " selected" : ""}>${esc(idleLabel(ms))}</option>`).join("")}</select></span></div>
      <div class="set-row col-picker"><span class="k">Columns<small>which columns show, and their order — saved in this browser</small></span><span class="v col-list">${colRows}</span></div>
    </div>`;
  $("signout2").addEventListener("click", signOut);
  const nt = $("notify-toggle");
  if (nt) {
    nt.addEventListener("change", async (e) => {
      if (!e.target.checked) {
        notifyOn = false;
        localStorage.setItem(NOTIFY_KEY, "0");
        renderSettings();
        return;
      }
      // The browser may refuse, and refusing is sticky: a toggle left on while
      // permission is denied would promise something that cannot happen.
      notifyOn = await enableNotifications();
      localStorage.setItem(NOTIFY_KEY, notifyOn ? "1" : "0");
      renderSettings();
    });
  }
  $("ended-toggle").addEventListener("change", (e) => {
    showEnded = Boolean(e.target.checked);
    localStorage.setItem(ENDED_KEY, showEnded ? "1" : "0");
    renderTabs(); renderSettings();
  });
  $("idle-select").addEventListener("change", (e) => {
    const ms = Number(e.target.value);
    idleHideAfter = IDLE_PRESETS_MS.includes(ms) ? ms : 0;
    localStorage.setItem(IDLE_KEY, String(idleHideAfter));
    renderTabs(); renderSettings();
  });
  const refresh = () => { renderSettings(); renderSessions(); };
  $("tab-settings").querySelectorAll("input[data-col]").forEach((el) => el.addEventListener("change", () => { toggleCol(el.dataset.col); refresh(); }));
  $("tab-settings").querySelectorAll("[data-mv-up]").forEach((b) => b.addEventListener("click", () => { moveCol(b.dataset.mvUp, -1); refresh(); }));
  $("tab-settings").querySelectorAll("[data-mv-down]").forEach((b) => b.addEventListener("click", () => { moveCol(b.dataset.mvDown, 1); refresh(); }));
}

// ---------- bottom bar: usage + platform ----------
function renderBottom() {
  $("botbar").hidden = false;
  const g = (lbl, pct, reset) => {
    const p = Math.max(0, Math.min(100, Math.round((pct || 0))));
    return `<div class="gauge"><span class="lbl">${lbl}</span><span class="track ${p >= 60 ? "warn" : ""}"><i data-w="${p}"></i></span><span class="pct">${p}%</span><span class="rst">${reset ? esc(relResetHint(reset)) : ""}</span></div>`;
  };
  const u = usage || {};
  const [pcls, ptxt] = platformClass(platform);
  const platHtml = (platform && platform.indicator) ? `<span class="plat ${pcls}"><span class="dot"></span>platform ${esc(ptxt)}</span>` : "";
  // `hidden N` is the one piece of the deleted summary strip that survives: the
  // visibility preferences filter silently, and without it the screen claims a
  // smaller fleet than it has (sessions-chrome.md § 2, test 3). Shown only when
  // something is hidden — a permanent zero trains the eye to skip the place where
  // the exception will appear — and only beside the table it describes (#548).
  const n = (activeTab === "sessions" && !detailId) ? sessions.length - preferenceVisible().length : 0;
  const hiddenHtml = n > 0 ? `<span class="hiddenn"><span class="lbl">hidden</span> ${n}</span>` : "";
  const html = `<div class="gauges">${g("5h", u.five_hour_pct, u.five_hour_reset)}${g("7d", u.seven_day_pct, u.seven_day_reset)}</div><span class="push"></span>${hiddenHtml}${watcherHtml()}${platHtml}`;
  if (!paint("botbar", html)) return;
  $("botbar").querySelectorAll("i[data-w]").forEach((i) => { i.style.width = i.dataset.w + "%"; });
}

// watcherHtml is the fleet's watcher indicator, beside the platform one: the
// dashboard's standing fleet-level state, which is where the TUI keeps the same
// answer. Silent while every watcher reports — a permanent green trains the eye
// to skip the place where the exception appears (sessions-chrome.md § 2) — and it
// names the machines when it speaks, so the operator does not have to open the
// Machines tab to learn where to go (#623).
//
// It says nothing at all before the first answer arrives: "not reported yet" is
// not the same as "not reporting", and only one of them is an alarm.
function watcherHtml() {
  if (!watcher || !watcher.machines) return "";
  const r = fleetAlarm(watcher.machines, Date.now());
  if (!r.alarm) return "";
  return `<span class="watch bad"><span class="dot"></span>watcher ${esc(fleetAlarmDetail(r.known, r.silent, r.unreadable))}</span>`;
}

// ---------- detail ----------
function field(k, v, cls = "") { return `<div class="field"><span class="k">${esc(k)}</span><span class="v ${cls}">${v}</span></div>`; }
function openDetail(id) {
  const s = byId.get(id); if (!s) return;
  detailId = id;
  const st = STATUSES.includes(s.status) ? s.status : "idle";
  const u = s.usage || {};
  const waiting = s.status === "waiting";
  const ctx = [
    field("Session id", esc(s.id), "mut"), field("User", esc(dash(s.user)), "mut"), field("Machine", esc(s.machine)),
    field("Directory", esc(dash(s.project_dir)), "mut"), field("Branch", s.git_branch ? esc(s.git_branch) : "—"),
    field("Model", esc(dash(s.model_short)), s.model ? "" : "mut"), field("Effort", esc(dash(s.effort)), s.effort ? "" : "mut"),
    field("Detail", detailText(s), hasCall(s) ? "call" : (waiting ? "wait" : "mut")),
    field("Remote control", s.remote_control ? "on ◉" : "off ○", "mut"),
    s.remote_url ? field("Remote", `<a href="${esc(s.remote_url)}" target="_blank" rel="noopener noreferrer">${esc(s.remote_url)}</a>`) : "",
    field("Last tool", esc(dash(s.last_tool)), "mut"),
  ].join("");
  const times = [field("Started", esc(dash(s.started_at)), "mut"), field("Last seen", esc(dash(s.last_seen_at)), "mut"), s.ended_at ? field("Ended", esc(s.ended_at), "mut") : ""].join("");
  const spark = (s.samples && s.samples.length) ? sparkSVG(s.samples, 300, 46) : '<span class="faint">no recent activity</span>';
  $("view-detail").innerHTML = `
    <button class="back" id="back">‹ sessions</button>
    <div class="d-hero st-${st}"><div class="h-main"><h1>${esc(s.name || s.id)}</h1>
      <div class="h-sub">${esc(s.machine)} · ${esc(s.project || "")}${s.git_branch ? " · " + esc(s.git_branch) : ""}</div>
      <div class="h-facts">
        <div class="fact"><span class="k">Tokens</span><span class="v">${humanTokens(totalTokens(u))}</span></div>
        <div class="fact"><span class="k">Output</span><span class="v">${humanTokens(u.output_tokens)}</span></div>
        <div class="fact"><span class="k">Seen</span><span class="v">${relAge(s.last_seen_at)}</span></div>
        <div class="fact"><span class="k">Remote ctl</span><span class="v">${s.remote_control ? "on" : "off"}</span></div>
      </div></div>
      <div class="h-right"><span class="pill st-${st}${hasCall(s) ? " call" : ""}"><span class="dot"></span>${st}</span></div></div>
    <div class="d-grid">
      <div class="card"><h3>Context</h3>${ctx}<h3>Timeline</h3>${times}</div>
      <div class="card"><h3>Tokens</h3>
        <div class="tok"><span class="k">Input</span><span>${humanTokens(u.input_tokens)}</span></div>
        <div class="tok"><span class="k">Output</span><span>${humanTokens(u.output_tokens)}</span></div>
        <div class="tok"><span class="k">Cache create</span><span>${humanTokens(u.cache_creation_tokens)}</span></div>
        <div class="tok"><span class="k">Cache read</span><span>${humanTokens(u.cache_read_tokens)}</span></div>
        <div class="tok total"><span class="k">Total</span><span>${humanTokens(totalTokens(u))}</span></div>
        <h3>Activity</h3><div class="act-block st-${st}">${spark}</div></div>
    </div>`;
  $("tab-" + activeTab).hidden = true;
  $("view-detail").hidden = false;
  syncFilterBar();
  $("back").addEventListener("click", closeDetail);
  window.scrollTo(0, 0);
}
function closeDetail() { detailId = null; $("view-detail").hidden = true; $("tab-" + activeTab).hidden = false; syncFilterBar(); window.scrollTo(0, 0); }

// ---------- loading ----------
async function loadSessions() {
  const data = await api("/api/sessions");
  sessions = Array.isArray(data) ? data : [];
  byId = new Map(sessions.map((s) => [s.id, s]));
  renderTabs();
  if (activeTab === "sessions" && !detailId) renderSessions();
  if (activeTab === "machines") renderMachines();
  if (usage || platform) renderBottom(); // the hidden count lives there now (#548)
  noteAttention();
}
async function loadMeta() {
  try { usage = await api("/api/usage"); } catch (e) { /* optional */ }
  try { platform = await api("/api/status"); } catch (e) { /* optional */ }
  try { ver = await api("/api/version"); renderVersion(); } catch (e) { /* optional */ }
  try { watcher = await api("/api/watcher"); if (activeTab === "machines") renderMachines(); } catch (e) { /* optional */ }
  renderBottom();
  if (activeTab === "settings" && settingsLoaded) renderSettings();
}

// The web shell is served by vigied, so the only build it can show is the
// daemon's — a topbar chip, full commit/build in its tooltip (#341).
function renderVersion() {
  const el = $("ver"); if (!el || !ver) return;
  el.textContent = "vigied " + (ver.version || "dev");
  el.title = [ver.commit && "commit " + ver.commit, ver.build_time && "built " + ver.build_time].filter(Boolean).join(" · ");
  el.hidden = false;
}

// ---------- live stream (fetch-based SSE, so the bearer header is sent) ----------
async function connectLive() {
  clearTimeout(liveRetry);
  if (liveCtrl) liveCtrl.abort();
  const ctrl = new AbortController(); liveCtrl = ctrl;
  lastHeardAt = Date.now(); // the silence window starts at the attempt, not at the first byte
  try {
    const res = await fetch("/api/events", { headers: authHeaders(), signal: ctrl.signal });
    if (res.status === 401) { onUnauthorized(); return; }
    if (!res.ok || !res.body) throw new Error("events " + res.status);
    setConn(true);
    const reader = res.body.getReader(), dec = new TextDecoder(); let buf = "";
    for (;;) {
      const { value, done } = await reader.read(); if (done) break;
      lastHeardAt = Date.now(); // any bytes, the keep-alive comment included, prove it is alive
      buf += dec.decode(value, { stream: true });
      let idx;
      while ((idx = buf.indexOf("\n\n")) >= 0) { const frame = buf.slice(0, idx); buf = buf.slice(idx + 2); if (frame.includes("event: sessions")) refreshSessions(); }
    }
    throw new Error("stream ended");
  } catch (e) {
    if (ctrl.signal.aborted) return; // superseded by a newer connect, which owns the state now
    liveCtrl = null;                 // nothing left to watch; the retry below owns reconnection
    setConn(false); liveRetry = setTimeout(connectLive, 5000);
  }
}

// tick is the periodic refresh, and it runs whether or not the stream is live.
//
// It used to be a fallback that the stream switched off, which looked reasonable
// and was not: `ended` and `stale` are never stored, the server derives them from
// the clock each time the list is read (internal/server/sessions.go). That
// transition changes no field, so the delta-gated fan-out has nothing to publish
// (#258), so a client that only listens never learns of it. A machine whose
// watcher had died stayed `working` on screen for as long as the tab was open,
// under a green `live` chip — and the relative ages froze with it (#538).
//
// The TUI has always polled for exactly this reason, and says so
// (internal/tui/model.go). One open tab now costs the server the same as one TUI.
function tick() {
  // A silent stream is indistinguishable from a dead one, and a suspended
  // machine's socket never errors: `read()` above simply blocks for minutes, so
  // the reconnect path is unreachable without a watchdog out here (#457).
  if (liveCtrl && streamIsSilent(lastHeardAt, Date.now())) {
    setConn(false);
    connectLive(); // aborts the silent stream on its way in
  }
  refreshSessions();
}
function startTicker() { clearInterval(tickTimer); tickTimer = setInterval(tick, REFRESH_MS); }
function stopTicker() { clearInterval(tickTimer); tickTimer = null; }
// streamLive and refreshFailed are the two things the chip reads. They fail
// independently — see `boardState` in lib.js for why conflating them hid the
// worse of the two (#673).
let streamLive = false, refreshFailed = false, everLoaded = false;
function setConn(live) { streamLive = live; paintConn(); }
function setRefreshFailed(failed) { refreshFailed = failed; paintConn(); }
function paintConn() {
  const { cls, text } = boardState(streamLive, refreshFailed);
  const el = $("conn");
  el.className = "chip conn " + cls;
  el.querySelector(".txt").textContent = text;
}

// refreshSessions is loadSessions with its outcome recorded rather than discarded. Every
// caller wants the same thing from a failure — say the board is not current, keep
// what is on screen — so none of them handles it, and none of them can forget to
// (#673, and internal/tui/sessions.go for the rule it borrows).
async function refreshSessions() {
  try {
    await loadSessions();
    everLoaded = true;
    setRefreshFailed(false);
  } catch (e) {
    // 401 is not staleness: `api` has already torn down and shown the gate.
    if (String(e && e.message) !== "unauthorized") setRefreshFailed(true);
  }
}

// ---------- calling the operator ----------
//
// The board answers "which session needs me" only while someone is looking at
// it. A phone or a second screen left on the dashboard is the case where nobody
// is — and that is the case the browser exists for, since the terminal is already
// in front of the operator. It could not say anything at all until #667.
//
// The rule is not invented here. Which sessions are calling is the daemon's
// answer (`attention`, ADR-0011) plus a raised call (ADR-0010), and *when* to
// fire is the one #665 settled across the terminal, the GNOME indicator and the
// README: on entry into the set, once per transition.
let notifyOn = localStorage.getItem(NOTIFY_KEY) === "1";
let attnSeen = new Set(), attnPrimed = false;

// The Notification API needs a **secure context**: https, or a localhost origin.
// That is not a detail here — deployment.md's own example binds a reachable
// interface over plain http, and a phone pointed at that is exactly the case this
// feature is for. On such an origin the browser hides the API entirely, and the
// operator must be told which of the two is missing rather than being shown a
// toggle that does nothing (#667).
function notifySupported() { return typeof Notification !== "undefined" && window.isSecureContext; }
function notifyBlockedReason() {
  if (typeof window !== "undefined" && !window.isSecureContext) {
    return "needs https (or a localhost address) — the browser hides notifications on a plain-http origin";
  }
  return "not available in this browser";
}

// noteAttention folds a fresh session list into the notification state, firing
// for each session that just entered the set.
//
// The bookkeeping happens whatever the settings say. Skipping it while
// notifications are off would leave a stale set behind, so switching them on
// would announce every session already blocked — the burst `primed` exists to
// prevent on the first poll.
function noteAttention() {
  const fresh = enteredAttention(sessions, attnSeen, attnPrimed);
  attnSeen = attentionIds(sessions);
  attnPrimed = true;

  if (!notifyOn || !notifySupported() || Notification.permission !== "granted") return;
  // Suppressed while the tab is in front, as the terminal suppresses while it has
  // focus: the operator is already looking. `document.hidden` is the closest the
  // platform gets — it is false for a visible tab behind another window — and
  // over-notifying is the safer side of that inaccuracy.
  if (!document.hidden) return;

  for (const s of fresh) {
    const n = new Notification("vigie — " + (s.name || s.id), { body: bodyFor(s), tag: "vigie-" + s.id });
    n.onclick = () => { window.focus(); openDetail(s.id); n.close(); };
  }
}

// The body says *why*, because a stalled turn, an API error and a raised call all
// want different things from the operator — the same reasoning the GNOME
// indicator's notification body follows.
function bodyFor(s) {
  if (hasCall(s)) return s.call_message ? String(s.call_message) : "calling you";
  return s.status ? String(s.status) : "needs you";
}

// jumpToAttention is the browser's `n`: the session blocked longest, or the
// oldest raised call ahead of it.
function jumpToAttention() {
  const id = nextAttention(sessions);
  if (!id) return;
  if (activeTab !== "sessions") switchTab("sessions");
  openDetail(id);
}

async function enableNotifications() {
  if (!notifySupported()) return false;
  // requestPermission must be called from a user gesture, which is why this hangs
  // off the toggle rather than firing on load.
  const p = Notification.permission === "granted" ? "granted" : await Notification.requestPermission();
  return p === "granted";
}

// ---------- auth / gate ----------
function onUnauthorized() { teardown(); localStorage.removeItem(TOKEN_KEY); token = ""; showGate(true); }
function teardown() { if (liveCtrl) liveCtrl.abort(); liveCtrl = null; clearTimeout(liveRetry); stopTicker(); clearInterval(metaTimer); metaTimer = null; }
function showGate(err) { $("gate").hidden = false; $("filterbar").hidden = true; $("gate-err").hidden = !err; $("signout").hidden = true; $("botbar").hidden = true; $("ver").hidden = true; setConn(false); $("token-input").focus(); }
function hideGate() { $("gate").hidden = true; $("signout").hidden = false; syncFilterBar(); }
function signOut() { teardown(); localStorage.removeItem(TOKEN_KEY); token = ""; showGate(false); }

async function start() {
  hideGate(); setConn(false);
  statsLoaded = false; settingsLoaded = false;
  renderTabs();
  // A first load that fails must not read as an empty fleet, which is what
  // discarding it did: the gate closed, the table drew nothing, and nothing said
  // the server had never answered (#673). The stream and the ticker still start —
  // the failure may be a restart, and recovering is their job.
  await refreshSessions();
  renderSessions();
  loadMeta();
  metaTimer = setInterval(loadMeta, 60000);
  startTicker();
  connectLive();
}

// ---------- wire up ----------
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape" && detailId) { closeDetail(); return; }
  // `n` jumps to the session that has been waiting longest, as it does in the
  // terminal (#261). Ignored while typing, or the filter box could never contain
  // the letter.
  const t = e.target;
  const typing = t && (t.tagName === "INPUT" || t.tagName === "SELECT" || t.tagName === "TEXTAREA" || t.isContentEditable);
  if (e.key === "n" && !typing && !e.ctrlKey && !e.metaKey && !e.altKey) { e.preventDefault(); jumpToAttention(); }
});
$("gate-form").addEventListener("submit", (e) => {
  e.preventDefault();
  const v = $("token-input").value.trim(); if (!v) return;
  token = v; localStorage.setItem(TOKEN_KEY, token); start();
});
$("signout").addEventListener("click", signOut);
$("group-select").addEventListener("change", (e) => {
  groupBy = GROUP_MODES.includes(e.target.value) ? e.target.value : "off";
  localStorage.setItem(GROUP_KEY, groupBy);
  if (activeTab === "sessions" && !detailId) renderSessions();
});
$("filter-input").addEventListener("input", (e) => {
  filter = e.target.value.trim();
  if (activeTab === "sessions" && !detailId) renderSessions(); else syncFilterBar();
});

renderTabs();
renderGroupSelect();
if (token) start(); else showGate(false);
