// #844. The same usage gauge was a different colour depending on which client was
// open: at 98 % the terminal drew it red and the browser amber — the colour that
// decides whether the operator reacts. The browser had one step at 60 % and no red
// at all; the terminal had 50/80.
//
// `usage.md` § 1bis now states the rule for both: amber from 50 %, red from 80 %.
// The terminal's half is `internal/tui/usage_render_test.go`; nothing keeps the two
// in step automatically, so the pair has to be reviewed together.
//
// Harness after `scoped_gauge.test.mjs`: fake only the I/O boundary and assert the
// HTML the operator would get.

import assert from "node:assert/strict";
import { test } from "node:test";

const APP = new URL("../../internal/web/static/app.js", import.meta.url);

let instance = 0;
const flush = async (n = 8) => { for (let i = 0; i < n; i++) await new Promise((r) => setImmediate(r)); };

function harness(usage) {
  const h = { writes: [], listeners: new Map(), intervals: [], now: Date.parse("2026-09-21T14:40:00Z") };
  const element = (id) => {
    const ds = {};
    const node = new Proxy(function () {}, {
      get: (_t, k) =>
        k === "addEventListener" ? (ev, fn) => h.listeners.set(`${id}:${ev}`, fn)
          : k === "querySelectorAll" ? () => []
            : k === "dataset" ? ds
              : k === "classList" || k === "style" ? node
                : k === "hidden" || k === "value" || k === "innerHTML" || k === "textContent" ? ""
                  : k === "children" || k === "options" ? []
                    : k === Symbol.toPrimitive || k === "toString" ? () => ""
                      : node,
      set: (_t, k, v) => { if (k === "innerHTML") h.writes.push({ id, html: String(v) }); return true; },
      apply: () => node,
      has: () => true,
    });
    return node;
  };
  const els = new Map();
  const byId = (id) => { if (!els.has(id)) els.set(id, element(id)); return els.get(id); };
  globalThis.document = {
    getElementById: byId, querySelectorAll: () => [], createElement: () => byId("<new>"),
    addEventListener: (ev, fn) => h.listeners.set(`document:${ev}`, fn),
    documentElement: byId("<root>"), body: byId("<body>"),
  };
  globalThis.window = globalThis;
  globalThis.localStorage = { getItem: (k) => (k === "vigie_token" ? "t0k3n" : null), setItem() {}, removeItem() {} };
  globalThis.matchMedia = () => ({ matches: false, addEventListener() {} });
  globalThis.scrollTo = () => {};
  globalThis.location = { origin: "http://localhost:8080", protocol: "http:", hostname: "localhost" };
  globalThis.Notification = undefined;
  globalThis.Date.now = () => h.now;
  globalThis.setInterval = (fn, ms) => { h.intervals.push({ fn, ms }); return h.intervals.length; };
  globalThis.clearInterval = () => {};
  globalThis.setTimeout = () => 0;
  globalThis.clearTimeout = () => {};
  globalThis.AbortController = class { constructor() { this.signal = { aborted: false }; } abort() { this.signal.aborted = true; } };
  globalThis.fetch = async (path) => {
    if (path === "/api/events") return { ok: true, status: 200, body: { getReader: () => ({ read: () => new Promise(() => {}) }) } };
    return { ok: true, status: 200, json: async () => (path === "/api/usage" ? usage : path === "/api/sessions" ? [] : {}) };
  };
  h.boot = async () => { await import(`${APP}?r=${++instance}`); await flush(); };
  h.lastBot = () => {
    const w = h.writes.filter((x) => x.id === "botbar");
    return w.length ? w[w.length - 1].html : "";
  };
  return h;
}

// trackClass returns the classes on the gauge whose figure is `pct`.
function trackClass(html, pct) {
  const gauges = html.split('class="gauge"');
  for (const g of gauges) {
    if (!g.includes(`>${pct}%<`)) continue;
    const m = g.match(/class="track([^"]*)"/);
    return m ? m[1].trim() : "";
  }
  throw new Error(`no gauge reading ${pct}% in:\n${html}`);
}

const at = (five, seven) => ({
  five_hour_pct: five, five_hour_reset: "2026-09-21T17:30:00Z",
  seven_day_pct: seven, seven_day_reset: "2026-09-23T03:00:00Z",
  fetched_at: "2026-09-21T14:40:00Z",
});

test("a gauge under 50% is plain", async () => {
  const h = harness(at(49, 1));
  await h.boot();
  assert.equal(trackClass(h.lastBot(), 49), "", "a gauge with nothing to decide must not be coloured");
});

test("a gauge from 50% is amber", async () => {
  const h = harness(at(50, 79));
  await h.boot();
  const html = h.lastBot();
  assert.equal(trackClass(html, 50), "warn", "amber starts at 50% (usage.md § 1bis)");
  assert.equal(trackClass(html, 79), "warn", "79% is still amber, not yet red");
});

// The defect itself: the browser had no red, so a limit about to stop the work
// looked like one merely worth planning around.
test("a gauge from 80% is red", async () => {
  const h = harness(at(80, 98));
  await h.boot();
  const html = h.lastBot();
  assert.equal(trackClass(html, 80), "hot", "red starts at 80% (usage.md § 1bis)");
  assert.equal(trackClass(html, 98), "hot",
    "at 98% the terminal draws red; the browser must not disagree about whether to react");
});
