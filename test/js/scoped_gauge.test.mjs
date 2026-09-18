// #840. Claude enforces a weekly limit scoped to one model, and the usage strip
// showed only `5h` and `7d`. Observed live, the scoped one sat at 98 % while the
// two on screen read 21 % and 73 % — the board looked comfortable while the limit
// that would stop the work was nearly full.
//
// This is the browser's half. The terminal's is in
// `internal/tui/scoped_gauge_test.go`, and nothing keeps the two in step: the
// column sets are pinned across clients (#550/#544), the gauges are not. Until
// that exists, each client's half is asserted in its own suite and the pair has to
// be reviewed together.
//
// Harness after `repaint.test.mjs`: fake only the I/O boundary and record what
// each render writes, so what is asserted is the HTML the operator would get.

import assert from "node:assert/strict";
import { test } from "node:test";

const APP = new URL("../../internal/web/static/app.js", import.meta.url);

let instance = 0;
const flush = async (n = 8) => { for (let i = 0; i < n; i++) await new Promise((r) => setImmediate(r)); };

function harness(usage) {
  const h = { writes: [], listeners: new Map(), intervals: [], now: Date.parse("2026-09-17T14:40:00Z") };
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

const base = {
  five_hour_pct: 21, five_hour_reset: "2026-09-17T17:30:00Z",
  seven_day_pct: 73, seven_day_reset: "2026-09-19T03:00:00Z",
  fetched_at: "2026-09-17T14:40:00Z",
};

test("the model-scoped limit is drawn, with the model's own name", async () => {
  const h = harness({ ...base, scoped: { label: "Fable", pct: 98, reset: "2026-09-19T03:00:00Z" } });
  await h.boot();
  const html = h.lastBot();
  assert.ok(html.includes("Fable"), `the scoped gauge is missing its label:\n${html}`);
  assert.ok(html.includes("98%"), `the scoped gauge is missing its figure:\n${html}`);
});

// Absent, not zero: a gauge permanently at nothing trains the eye to skip the place
// where the exception appears, which is why `hidden N` is shown only when it has a
// value.
test("nothing is drawn when no limit is scoped to a model", async () => {
  const h = harness({ ...base });
  await h.boot();
  const html = h.lastBot();
  assert.ok(!html.includes("Fable"), `a gauge was invented from a payload with none:\n${html}`);
  assert.equal((html.match(/class="gauge"/g) || []).length, 2, "the strip must keep exactly its two gauges");
});

// The reset is written once because the two share a window — not because it is
// shorter. If the endpoint ever reports two instants, both are printed; dropping
// one would then state something false about the other.
test("the weekly reset is written once, and only while the two instants match", async () => {
  const shared = harness({ ...base, scoped: { label: "Fable", pct: 98, reset: "2026-09-19T03:00:00Z" } });
  await shared.boot();
  assert.equal((shared.lastBot().match(/class="rst">[^<]+</g) || []).length, 2,
    "want two resets: the five-hour one and the shared weekly one");

  const diverged = harness({ ...base, scoped: { label: "Fable", pct: 98, reset: "2026-09-20T09:00:00Z" } });
  await diverged.boot();
  assert.equal((diverged.lastBot().match(/class="rst">[^<]+</g) || []).length, 3,
    "the two weekly windows reset at different times, so both must be shown");
});
