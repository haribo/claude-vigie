"use strict";
// claude-vigie — read-only web dashboard, a browser mirror of the terminal UI.
// It only reads the API (observe-only). The operator's token lives in localStorage
// and is sent as a bearer token; all data is HTML-escaped before it reaches the
// DOM, since a session's title/branch/activity are transcript-derived and the
// token makes DOM-based XSS a real risk (issue #161). Dynamic sizes are applied
// via CSSOM (element.style), never inline attributes, to keep the strict CSP.

const TOKEN_KEY = "cf_token";
const STATUSES = ["working", "thinking", "waiting", "idle", "error", "ended"];
const RANK = { working: 0, thinking: 1, waiting: 2, idle: 3, error: 4, ended: 5 };

let token = localStorage.getItem(TOKEN_KEY) || "";
let sessions = [], byId = new Map();
let usage = null, platform = null, stats = null, settings = null;
let activeTab = "sessions", detailId = null, showEnded = false;
let sortKey = "seen", sortDir = 1;           // 1 = descending
let statsPeriod = "Week", statsLoaded = false, settingsLoaded = false;
let liveCtrl = null, liveRetry = null, pollTimer = null, metaTimer = null;

const $ = (id) => document.getElementById(id);
function esc(s) {
  return String(s == null ? "" : s).replaceAll("&", "&amp;").replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;").replaceAll('"', "&quot;").replaceAll("'", "&#39;");
}
const dash = (v) => (v === "" || v == null) ? "-" : v;
const trim = (x) => x.toFixed(1).replace(/\.0$/, "");
function humanTokens(n) { n = Number(n) || 0; if (n >= 1e6) return trim(n / 1e6) + "M"; if (n >= 1e3) return trim(n / 1e3) + "k"; return String(n); }
function ageSec(rfc) { const t = Date.parse(rfc); return Number.isNaN(t) ? Infinity : Math.max(0, (Date.now() - t) / 1000); }
function relAge(rfc) {
  const s = ageSec(rfc); if (!Number.isFinite(s)) return "-";
  if (s < 60) return Math.floor(s) + "s";
  const m = Math.floor(s / 60); if (m < 60) return m + "m";
  const h = Math.floor(m / 60); if (h < 24) return h + "h";
  return Math.floor(h / 24) + "d";
}
function relResetHint(rfc) {
  const t = Date.parse(rfc); if (Number.isNaN(t)) return "";
  let s = Math.floor((t - Date.now()) / 1000); if (s <= 0) return "resets soon";
  const d = Math.floor(s / 86400); s -= d * 86400; const h = Math.floor(s / 3600); s -= h * 3600; const m = Math.floor(s / 60);
  if (d) return `resets in ${d}d ${h}h`; if (h) return `resets in ${h}h ${m}m`; return `resets in ${m}m`;
}
const shortModel = (m) => (m || "").replace(/^claude-/, "");
function projectName(dir) { if (!dir) return "-"; const p = dir.replace(/\/+$/, "").split("/"); return p[p.length - 1] || dir; }
const totalTokens = (u) => (u.input_tokens || 0) + (u.output_tokens || 0) + (u.cache_creation_tokens || 0) + (u.cache_read_tokens || 0);

function sparkSVG(data, w = 72, h = 18) {
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
  const waiting = sessions.filter((s) => s.status === "waiting").length;
  $("tabbar").innerHTML = TABS.map((t) => {
    const badge = (t.id === "sessions" && waiting) ? `<span class="badge">${waiting}</span>` : "";
    return `<button class="tab ${t.id === activeTab ? "active" : ""}" data-tab="${t.id}">${t.label}${badge}</button>`;
  }).join("");
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
const COLS = [
  { key: "name", label: "Session", cmp: (a, b) => (a.title || a.id).localeCompare(b.title || b.id) },
  { key: "machine", label: "Machine", cmp: (a, b) => a.machine.localeCompare(b.machine) },
  { key: "project", label: "Project", cmp: (a, b) => projectName(a.project_dir).localeCompare(projectName(b.project_dir)) },
  { key: "branch", label: "Branch", cmp: (a, b) => (a.git_branch || "").localeCompare(b.git_branch || "") },
  { key: "model", label: "Model", cmp: (a, b) => (a.model || "").localeCompare(b.model || "") },
  { key: "tokens", label: "Tokens", num: true, cmp: (a, b) => totalTokens(a.usage || {}) - totalTokens(b.usage || {}) },
  { key: "seen", label: "Seen", num: true, cmp: (a, b) => ageSec(b.last_seen_at) - ageSec(a.last_seen_at) },
  { key: "activity", label: "Activity", nosort: true },
  { key: "rc", label: "RC", cmp: (a, b) => (a.remote_control === b.remote_control ? 0 : a.remote_control ? -1 : 1) },
  { key: "status", label: "Status", cmp: (a, b) => RANK[a.status] - RANK[b.status] },
  { key: "doing", label: "Doing", nosort: true },
  { key: "act", label: "", nosort: true },
];
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
  const cnts = STATUSES.map((k) => `<span class="cnt st-${k}"><span class="dot"></span><b>${counts[k] || 0}</b><small>${k}</small></span>`).join("");
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
  const heads = COLS.map((c) => {
    const sorted = c.key === sortKey;
    const arrow = sorted ? `<span class="arrow">${sortDir === 1 ? "▼" : "▲"}</span>` : "";
    const cls = [c.num ? "num" : "", c.nosort ? "nosort" : "", sorted ? "sorted" : ""].filter(Boolean).join(" ");
    return `<th class="${cls}" ${c.nosort ? "" : `data-sort="${c.key}"`}>${c.label}${arrow}</th>`;
  }).join("");
  const rows = visibleSessions().map((s) => {
    const st = STATUSES.includes(s.status) ? s.status : "idle";
    const attn = s.status === "waiting" ? " attn" : "";
    const rc = s.remote_control ? '<span class="rc-on" title="Remote control on">◉</span>' : '<span class="rc-off" title="Remote control off">○</span>';
    const code = (s.status === "error" && s.api_error_status) ? ` <span class="code">${s.api_error_status}</span>` : "";
    const name = esc(s.title || s.id), proj = esc(projectName(s.project_dir)), branch = esc(dash(s.git_branch));
    const doing = s.activity ? esc(s.activity) : "-", doingCls = s.status === "waiting" ? "doing wait" : "doing";
    return `<tr class="st-${st}${attn}" data-id="${esc(s.id)}" tabindex="0">
      <td class="name" title="${name}"><span class="nm">${name}</span></td>
      <td class="dim">${esc(s.machine)}</td>
      <td class="proj" title="${proj}">${proj}</td>
      <td class="branch dim" title="${branch}">${branch}</td>
      <td class="${s.model ? "dim" : "faint"}">${esc(dash(shortModel(s.model)))}</td>
      <td class="num">${humanTokens(totalTokens(s.usage || {}))}</td>
      <td class="num dim">${relAge(s.last_seen_at)}</td>
      <td>${sparkSVG(s.samples)}</td>
      <td>${rc}</td>
      <td><span class="pill st-${st}"><span class="dot"></span>${st}${code}</span></td>
      <td class="${doingCls}" title="${doing}">${doing}</td>
      <td><button class="det-btn" aria-label="Open detail" title="Open detail"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M9 6l6 6-6 6"/></svg></button></td>
    </tr>`;
  }).join("");
  const empty = visibleSessions().length ? "" : '<div class="empty">No sessions in view.</div>';
  $("tab-sessions").innerHTML = renderSummary() +
    `<div class="table-wrap"><div class="table-scroll"><table><thead><tr>${heads}</tr></thead><tbody>${rows}</tbody></table></div>${empty}</div>`;
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
    return `<div class="mach">
      <div class="mach-head"><span class="nm">${esc(a.name)}</span><span class="u">${esc(a.user)}</span></div>
      <div class="mach-row"><span class="big">${a.n}<small>${a.n === 1 ? "session" : "sessions"}</small></span></div>
      <div class="mach-brk">${brk}</div>
      <div class="mach-foot"><span>output <b>${humanTokens(a.out)}</b></span><span>last seen <b>${relAge(a.seen)}</b></span></div>
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
  $("tab-settings").innerHTML = `
    <div class="settings">
      <div class="set-note"><span>ℹ</span><span>Settings are <b>read-only</b> from the web client — claude-vigie is observe-only. Change them on the daemon.</span></div>
      <div class="set-row"><span class="k">Server<small>the daemon this dashboard is served by</small></span><span class="v">${esc(location.origin)}</span></div>
      <div class="set-row"><span class="k">Session retention<small>how long closed sessions are kept</small></span><span class="v">${esc(retention)}</span></div>
      <div class="set-row"><span class="k">Platform status<small>polled from status.claude.com</small></span><span class="v ${pcls === "ok" ? "ok" : ""}">● ${esc(ptxt)}</span></div>
      <div class="set-row"><span class="k">Token<small>stored in this browser, sent as a bearer token</small></span><span class="v">connected <button class="signout2" id="signout2">sign out</button></span></div>
    </div>`;
  $("signout2").addEventListener("click", signOut);
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
  const doingWait = s.status === "waiting";
  const ctx = [
    field("Session id", esc(s.id), "mut"), field("User", esc(dash(s.user)), "mut"), field("Machine", esc(s.machine)),
    field("Directory", esc(dash(s.project_dir)), "mut"), field("Branch", s.git_branch ? esc(s.git_branch) : "—"),
    field("Model", esc(dash(shortModel(s.model))), s.model ? "" : "mut"), field("Doing", esc(dash(s.activity)), doingWait ? "wait" : "mut"),
    field("Remote control", s.remote_control ? "on ◉" : "off ○", "mut"), field("Last tool", esc(dash(s.last_tool)), "mut"),
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
      <div class="h-right"><span class="pill st-${st}"><span class="dot"></span>${st}${code}</span></div></div>
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
  renderBottom();
  if (activeTab === "settings" && settingsLoaded) renderSettings();
}

// ---------- live stream (fetch-based SSE, so the bearer header is sent) ----------
async function connectLive() {
  clearTimeout(liveRetry);
  if (liveCtrl) liveCtrl.abort();
  const ctrl = new AbortController(); liveCtrl = ctrl;
  try {
    const res = await fetch("/api/events", { headers: authHeaders(), signal: ctrl.signal });
    if (res.status === 401) { onUnauthorized(); return; }
    if (!res.ok || !res.body) throw new Error("events " + res.status);
    setConn(true); stopPolling();
    const reader = res.body.getReader(), dec = new TextDecoder(); let buf = "";
    for (;;) {
      const { value, done } = await reader.read(); if (done) break;
      buf += dec.decode(value, { stream: true });
      let idx;
      while ((idx = buf.indexOf("\n\n")) >= 0) { const frame = buf.slice(0, idx); buf = buf.slice(idx + 2); if (frame.includes("event: sessions")) loadSessions().catch(() => {}); }
    }
    throw new Error("stream ended");
  } catch (e) {
    if (ctrl.signal.aborted) return;
    setConn(false); startPolling(); liveRetry = setTimeout(connectLive, 5000);
  }
}
function startPolling() { if (!pollTimer) pollTimer = setInterval(() => loadSessions().catch(() => {}), 5000); }
function stopPolling() { clearInterval(pollTimer); pollTimer = null; }
function setConn(live) { const el = $("conn"); el.className = "chip conn " + (live ? "live" : "down"); el.querySelector(".txt").textContent = live ? "live" : "reconnecting…"; }

// ---------- auth / gate ----------
function onUnauthorized() { teardown(); localStorage.removeItem(TOKEN_KEY); token = ""; showGate(true); }
function teardown() { if (liveCtrl) liveCtrl.abort(); liveCtrl = null; clearTimeout(liveRetry); stopPolling(); clearInterval(metaTimer); metaTimer = null; }
function showGate(err) { $("gate").hidden = false; $("gate-err").hidden = !err; $("signout").hidden = true; $("botbar").hidden = true; setConn(false); $("token-input").focus(); }
function hideGate() { $("gate").hidden = true; $("signout").hidden = false; }
function signOut() { teardown(); localStorage.removeItem(TOKEN_KEY); token = ""; showGate(false); }

async function start() {
  hideGate(); setConn(false);
  statsLoaded = false; settingsLoaded = false;
  renderTabs();
  await loadSessions();
  loadMeta();
  metaTimer = setInterval(loadMeta, 60000);
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
