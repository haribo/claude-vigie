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

// detailText is the Detail cell's content: a raised call takes it, because it is
// the reason the row is pulsing and it outranks the tool that ran last.
export function detailText(s) {
  if (hasCall(s)) return s.call_message ? esc(s.call_message) : "called you";
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

// LEGACY_COLS maps a key saved under an older name to its current one. Without
// it, a layout saved before a rename silently loses that column.
export const LEGACY_COLS = { doing: "detail" };

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
