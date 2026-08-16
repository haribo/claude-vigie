// claude-vigie — read-only web dashboard, a browser mirror of the terminal UI.
// It only reads the API (observe-only). The operator's token lives in localStorage
// and is sent as a bearer token; all data is HTML-escaped before it reaches the
// DOM, since a session's title/branch/activity are transcript-derived and the
// token makes DOM-based XSS a real risk (issue #161). Dynamic sizes are applied
// via CSSOM (element.style), never inline attributes, to keep the strict CSP.

import {
  esc, dash, trim, hasCall, detailText, humanTokens, ageSec, relAge, relResetHint,
  shortModel, projectName, totalTokens, sparkSVG, migrateKeys, fullColOrder, colHidden, rank,
  adoptLegacyKey, needsAttention, attentionCount, streamIsSilent, REFRESH_MS,
} from "./lib.js";

// Both keys were named for the old brand. They hold live state — a signed-in
// token and a column layout — so the old name is read once and carried over
// rather than dropped (adoptLegacyKey, #478).
const TOKEN_KEY = "vigie_token";
// Kept identical to docs/design/session-status.md § 1 and internal/status — a Go
// test reads this literal and fails on any drift (#423).
const STATUSES = ["working", "thinking", "compacting", "waiting", "stalled", "idle", "error", "stale", "ended"];

let token = adoptLegacyKey(localStorage, "cf_token", TOKEN_KEY) || "";
let sessions = [], byId = new Map();
let usage = null, platform = null, stats = null, settings = null, ver = null, watcher = null;
let activeTab = "sessions", detailId = null, showEnded = false;
let sortKey = "seen", sortDir = 1;           // 1 = descending
let statsPeriod = "Week", statsLoaded = false, settingsLoaded = false;
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
function switchTab(id) {
  activeTab = id; detailId = null; $("view-detail").hidden = true;
  ["sessions", "stats", "machines", "settings"].forEach((t) => $("tab-" + t).hidden = (t !== id));
  if (id === "machines") renderMachines();
  if (id === "stats") { if (!statsLoaded) loadStats(); else renderStats(); }
  if (id === "settings") { if (!settingsLoaded) loadSettings(); else renderSettings(); }
  renderTabs(); window.scrollTo(0, 0);
}

// ---------- Sessions ----------
// Each column carries how to render its cell, so the header and the rows are both
// driven by the (operator-ordered, filtered) column list — see activeCols (#309).
const COLS = [
  { key: "name", label: "Session", cmp: (a, b) => (a.title || a.id).localeCompare(b.title || b.id),
    cell: (s) => { const n = esc(s.title || s.id); return `<td class="name" title="${n}"><span class="nm">${n}</span></td>`; } },
  { key: "machine", label: "Machine", cmp: (a, b) => a.machine.localeCompare(b.machine),
    cell: (s) => `<td class="dim">${esc(s.machine)}</td>` },
  { key: "project", label: "Project", cmp: (a, b) => projectName(a.project_dir).localeCompare(projectName(b.project_dir)),
    cell: (s) => { const p = esc(projectName(s.project_dir)); return `<td class="proj" title="${p}">${p}</td>`; } },
  { key: "branch", label: "Branch", cmp: (a, b) => (a.git_branch || "").localeCompare(b.git_branch || ""),
    cell: (s) => { const b = esc(dash(s.git_branch)); return `<td class="branch dim" title="${b}">${b}</td>`; } },
  { key: "model", label: "Model", cmp: (a, b) => (a.model || "").localeCompare(b.model || ""),
    cell: (s) => `<td class="${s.model ? "dim" : "faint"}">${esc(dash(shortModel(s.model)))}</td>` },
  { key: "effort", label: "Effort", cmp: (a, b) => (a.effort || "").localeCompare(b.effort || ""),
    cell: (s) => `<td class="${s.effort ? "dim" : "faint"}">${esc(dash(s.effort))}</td>` },
  { key: "tokens", label: "Tokens", num: true, cmp: (a, b) => totalTokens(a.usage || {}) - totalTokens(b.usage || {}),
    cell: (s) => `<td class="num">${humanTokens(totalTokens(s.usage || {}))}</td>` },
  { key: "seen", label: "Seen", num: true, cmp: (a, b) => ageSec(b.last_seen_at) - ageSec(a.last_seen_at),
    cell: (s) => `<td class="num dim">${relAge(s.last_seen_at)}</td>` },
  { key: "activity", label: "Activity", nosort: true,
    cell: (s) => `<td>${sparkSVG(s.samples)}</td>` },
  { key: "rc", label: "RC", cmp: (a, b) => (a.remote_control === b.remote_control ? 0 : a.remote_control ? -1 : 1),
    cell: (s) => `<td>${s.remote_control ? '<span class="rc-on" title="Remote control on">◉</span>' : '<span class="rc-off" title="Remote control off">○</span>'}</td>` },
  { key: "status", label: "Status", cmp: (a, b) => rank(a.status) - rank(b.status),
    cell: (s) => { const st = STATUSES.includes(s.status) ? s.status : "idle"; const code = (s.status === "error" && s.api_error_status) ? ` <span class="code">${s.api_error_status}</span>` : ""; return `<td><span class="pill st-${st}${hasCall(s) ? " call" : ""}"><span class="dot"></span>${st}${code}</span></td>`; } },
  { key: "detail", label: "Detail", nosort: true,
    cell: (s) => { const d = detailText(s); const cls = hasCall(s) ? "detail call" : (s.status === "waiting" ? "detail wait" : "detail"); return `<td class="${cls}" title="${d}">${d}</td>`; } },
  { key: "act", label: "", nosort: true,
    cell: () => `<td><button class="det-btn" aria-label="Open detail" title="Open detail"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M9 6l6 6-6 6"/></svg></button></td>` },
];

// COLS_KEY holds the operator's column layout for this browser, {order, hidden}:
// the display order of ALL columns plus the hidden set. Client-local like the
// token. Hiding a column keeps its position — only its visibility changes (#315).
const COLS_KEY = "vigie_columns";
const MANDATORY_COLS = new Set(["name", "status"]);
const COL_KEYS = COLS.map((c) => c.key);


function loadLayout() {
  try {
    const v = JSON.parse(adoptLegacyKey(localStorage, "cf_columns", COLS_KEY) || "null");
    // Migrate the old visible-only array form to {order, hidden}.
    if (Array.isArray(v)) return { order: v.slice(), hidden: COL_KEYS.filter((k) => !v.includes(k) && !MANDATORY_COLS.has(k)) };
    if (v && Array.isArray(v.order)) return { order: migrateKeys(v.order), hidden: migrateKeys(Array.isArray(v.hidden) ? v.hidden : []) };
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
function visibleSessions() {
  const list = showEnded ? sessions : sessions.filter((s) => s.status !== "ended");
  const col = COLS.find((c) => c.key === sortKey);
  return col && col.cmp ? [...list].sort((a, b) => col.cmp(a, b) * sortDir) : list;
}

function renderSummary() {
  const counts = {}; sessions.forEach((s) => counts[s.status] = (counts[s.status] || 0) + 1);
  const totalOut = sessions.reduce((n, s) => n + (s.usage ? s.usage.output_tokens || 0 : 0), 0);
  const rc = sessions.filter((s) => s.remote_control).length;
  const shown = showEnded ? sessions : sessions.filter((s) => s.status !== "ended");
  const hidden = sessions.length - shown.length;
  let agg = []; shown.forEach((s) => (s.samples || []).forEach((v, i) => { agg[i] = (agg[i] || 0) + v; }));
  const calling = sessions.filter(hasCall).length;
  // Shown only when non-zero, and first: a call is explicit where every other
  // count is a status vigie inferred. It borrows the `waiting` colour rather
  // than introducing a new one.
  const callCnt = calling ? `<span class="cnt st-waiting"><span class="dot"></span><b>${calling}</b><small>call</small></span>` : "";
  const cnts = callCnt + STATUSES.map((k) => `<span class="cnt st-${k}"><span class="dot"></span><b>${counts[k] || 0}</b><small>${k}</small></span>`).join("");
  return `<div class="summary">
    <div class="grp">${cnts}</div>
    <div class="div"></div>
    <span class="metric"><span class="lbl">out</span><b>${humanTokens(totalOut)}</b></span>
    <span class="metric rc"><span class="lbl">rc</span><b>◉ ${rc}</b></span>
    <span class="metric"><span class="lbl">activity</span><span class="st-working">${sparkSVG(agg, 90, 16)}</span></span>
    <span class="push"></span>
    <button class="metric hiddenm ${showEnded ? "on" : ""}" id="hidden-toggle" title="Show or hide ended sessions"><span class="lbl">${showEnded ? "showing all" : "hidden"}</span> ${hidden}</button>
  </div>`;
}

function renderSessions() {
  const cols = activeCols();
  const heads = cols.map((c) => {
    const sorted = c.key === sortKey;
    const arrow = sorted ? `<span class="arrow">${sortDir === 1 ? "▼" : "▲"}</span>` : "";
    const cls = [c.num ? "num" : "", c.nosort ? "nosort" : "", sorted ? "sorted" : ""].filter(Boolean).join(" ");
    return `<th class="${cls}" ${c.nosort ? "" : `data-sort="${c.key}"`}>${c.label}${arrow}</th>`;
  }).join("");
  const rows = visibleSessions().map((s) => {
    const st = STATUSES.includes(s.status) ? s.status : "idle";
    // A call reuses the attention mechanism — the left border in --st — and adds
    // a faint tint of the same colour. No new colour anywhere (ADR-0010).
    // The set is consulted, never restated: it is shared with the TUI and the
    // GNOME indicator, and restating it here is how `error` went unmarked (#538).
    const attn = needsAttention(s) ? " attn" : "";
    const call = hasCall(s) ? " call" : "";
    const cells = cols.map((c) => c.cell(s)).join("");
    return `<tr class="st-${st}${attn}${call}" data-id="${esc(s.id)}" tabindex="0">${cells}</tr>`;
  }).join("");
  const empty = visibleSessions().length ? "" : '<div class="empty">No sessions in view.</div>';
  const html = renderSummary() +
    `<div class="table-wrap"><div class="table-scroll"><table><thead><tr>${heads}</tr></thead><tbody>${rows}</tbody></table></div>${empty}</div>`;
  if (!paint("tab-sessions", html)) return; // nothing on screen would change; listeners are still bound
  $("hidden-toggle").addEventListener("click", () => { showEnded = !showEnded; renderSessions(); });
  $("tab-sessions").querySelectorAll("th[data-sort]").forEach((th) => th.addEventListener("click", () => {
    const k = th.dataset.sort; if (k === sortKey) sortDir = -sortDir; else { sortKey = k; sortDir = 1; } renderSessions();
  }));
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
    return `<div class="mach">
      <div class="mach-head"><span class="nm">${esc(a.name)}</span><span class="u">${esc(a.user)}</span></div>
      <div class="mach-row"><span class="big">${a.n}<small>${a.n === 1 ? "session" : "sessions"}</small></span></div>
      <div class="mach-brk">${brk}</div>
      <div class="mach-foot"><span>output <b>${humanTokens(a.out)}</b></span><span>watcher <b>${esc(wver)}</b></span><span>last seen <b>${relAge(a.seen)}</b></span></div>
    </div>`;
  }).join("");
  $("tab-machines").innerHTML = sessions.length ? `<div class="mach-grid">${cards}</div>` : '<div class="empty">No machines reporting.</div>';
}

// ---------- Stats ----------
async function loadStats() {
  try { stats = await api("/api/stats"); statsLoaded = true; renderStats(); } catch (e) { /* stats optional */ }
}
const PERIOD_DAYS = { Day: 1, Week: 7, Month: 30, Year: 365, Total: Infinity };
function renderStats() {
  if (!stats) { $("tab-stats").innerHTML = '<div class="muted-note">No stats yet.</div>'; return; }
  const cutoff = Date.now() - PERIOD_DAYS[statsPeriod] * 86400000;
  const daily = (stats.daily || []).filter((d) => { const t = Date.parse(d.day + "T00:00:00Z"); return Number.isNaN(t) || t >= cutoff; });
  const out = daily.reduce((n, d) => n + (d.output_tokens || 0), 0);
  const work = daily.reduce((n, d) => n + (d.working_seconds || 0), 0);
  const wait = daily.reduce((n, d) => n + (d.waiting_seconds || 0), 0);
  const idle = daily.reduce((n, d) => n + (d.idle_seconds || 0), 0);
  const active = work + wait + idle;
  const machines = new Set(sessions.map((s) => s.machine)).size;
  const periods = Object.keys(PERIOD_DAYS).map((p) => `<button class="period ${p === statsPeriod ? "active" : ""}" data-p="${p}">${p}</button>`).join("");
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
      <div class="card"><h3>Time by status (this ${statsPeriod.toLowerCase()})</h3>
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
      <div class="set-row col-picker"><span class="k">Columns<small>which columns show, and their order — saved in this browser</small></span><span class="v col-list">${colRows}</span></div>
    </div>`;
  $("signout2").addEventListener("click", signOut);
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
  $("botbar").innerHTML = `<div class="gauges">${g("5h", u.five_hour_pct, u.five_hour_reset)}${g("7d", u.seven_day_pct, u.seven_day_reset)}</div><span class="push"></span>${platHtml}`;
  $("botbar").querySelectorAll("i[data-w]").forEach((i) => { i.style.width = i.dataset.w + "%"; });
}

// ---------- detail ----------
function field(k, v, cls = "") { return `<div class="field"><span class="k">${esc(k)}</span><span class="v ${cls}">${v}</span></div>`; }
function openDetail(id) {
  const s = byId.get(id); if (!s) return;
  detailId = id;
  const st = STATUSES.includes(s.status) ? s.status : "idle";
  const u = s.usage || {};
  const code = (s.status === "error" && s.api_error_status) ? ` <span class="code">${s.api_error_status}</span>` : "";
  const waiting = s.status === "waiting";
  const ctx = [
    field("Session id", esc(s.id), "mut"), field("User", esc(dash(s.user)), "mut"), field("Machine", esc(s.machine)),
    field("Directory", esc(dash(s.project_dir)), "mut"), field("Branch", s.git_branch ? esc(s.git_branch) : "—"),
    field("Model", esc(dash(shortModel(s.model))), s.model ? "" : "mut"), field("Effort", esc(dash(s.effort)), s.effort ? "" : "mut"),
    field("Detail", detailText(s), hasCall(s) ? "call" : (waiting ? "wait" : "mut")),
    field("Remote control", s.remote_control ? "on ◉" : "off ○", "mut"),
    s.remote_url ? field("Remote", `<a href="${esc(s.remote_url)}" target="_blank" rel="noopener noreferrer">${esc(s.remote_url)}</a>`) : "",
    field("Last tool", esc(dash(s.last_tool)), "mut"),
  ].join("");
  const times = [field("Started", esc(dash(s.started_at)), "mut"), field("Last seen", esc(dash(s.last_seen_at)), "mut"), s.ended_at ? field("Ended", esc(s.ended_at), "mut") : ""].join("");
  const spark = (s.samples && s.samples.length) ? sparkSVG(s.samples, 300, 46) : '<span class="faint">no recent activity</span>';
  $("view-detail").innerHTML = `
    <button class="back" id="back">‹ sessions</button>
    <div class="d-hero st-${st}"><div class="h-main"><h1>${esc(s.title || s.id)}</h1>
      <div class="h-sub">${esc(s.machine)} · ${esc(projectName(s.project_dir))}${s.git_branch ? " · " + esc(s.git_branch) : ""}</div>
      <div class="h-facts">
        <div class="fact"><span class="k">Tokens</span><span class="v">${humanTokens(totalTokens(u))}</span></div>
        <div class="fact"><span class="k">Output</span><span class="v">${humanTokens(u.output_tokens)}</span></div>
        <div class="fact"><span class="k">Seen</span><span class="v">${relAge(s.last_seen_at)}</span></div>
        <div class="fact"><span class="k">Remote ctl</span><span class="v">${s.remote_control ? "on" : "off"}</span></div>
      </div></div>
      <div class="h-right"><span class="pill st-${st}${hasCall(s) ? " call" : ""}"><span class="dot"></span>${st}${code}</span></div></div>
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
  $("back").addEventListener("click", closeDetail);
  window.scrollTo(0, 0);
}
function closeDetail() { detailId = null; $("view-detail").hidden = true; $("tab-" + activeTab).hidden = false; window.scrollTo(0, 0); }

// ---------- loading ----------
async function loadSessions() {
  const data = await api("/api/sessions");
  sessions = Array.isArray(data) ? data : [];
  byId = new Map(sessions.map((s) => [s.id, s]));
  renderTabs();
  if (activeTab === "sessions" && !detailId) renderSessions();
  if (activeTab === "machines") renderMachines();
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
      while ((idx = buf.indexOf("\n\n")) >= 0) { const frame = buf.slice(0, idx); buf = buf.slice(idx + 2); if (frame.includes("event: sessions")) loadSessions().catch(() => {}); }
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
  loadSessions().catch(() => {});
}
function startTicker() { clearInterval(tickTimer); tickTimer = setInterval(tick, REFRESH_MS); }
function stopTicker() { clearInterval(tickTimer); tickTimer = null; }
function setConn(live) { const el = $("conn"); el.className = "chip conn " + (live ? "live" : "down"); el.querySelector(".txt").textContent = live ? "live" : "reconnecting…"; }

// ---------- auth / gate ----------
function onUnauthorized() { teardown(); localStorage.removeItem(TOKEN_KEY); token = ""; showGate(true); }
function teardown() { if (liveCtrl) liveCtrl.abort(); liveCtrl = null; clearTimeout(liveRetry); stopTicker(); clearInterval(metaTimer); metaTimer = null; }
function showGate(err) { $("gate").hidden = false; $("gate-err").hidden = !err; $("signout").hidden = true; $("botbar").hidden = true; $("ver").hidden = true; setConn(false); $("token-input").focus(); }
function hideGate() { $("gate").hidden = true; $("signout").hidden = false; }
function signOut() { teardown(); localStorage.removeItem(TOKEN_KEY); token = ""; showGate(false); }

async function start() {
  hideGate(); setConn(false);
  statsLoaded = false; settingsLoaded = false;
  renderTabs();
  await loadSessions();
  loadMeta();
  metaTimer = setInterval(loadMeta, 60000);
  startTicker();
  connectLive();
}

// ---------- wire up ----------
document.addEventListener("keydown", (e) => { if (e.key === "Escape" && detailId) closeDetail(); });
$("gate-form").addEventListener("submit", (e) => {
  e.preventDefault();
  const v = $("token-input").value.trim(); if (!v) return;
  token = v; localStorage.setItem(TOKEN_KEY, token); start().catch(() => {});
});
$("signout").addEventListener("click", signOut);

renderTabs();
if (token) start().catch(() => {}); else showGate(false);
