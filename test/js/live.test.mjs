// #538. What the dashboard does over time, tested against the shipped app.js.
//
// `boot.test.mjs` states — correctly — that behaviour asserted against its stub
// would be asserted against a fiction: that stub answers every property access
// with a permissive object, so a render assertion there proves nothing. This file
// is not that. It fakes only the I/O boundary — the clock, the timers, `fetch`
// and the stream body — and asserts what app.js *does* with them: which requests
// it makes, and when. Those are not fictions; they are the whole defect.
//
// The defect: `ended` and `stale` are derived from the clock at read time
// (internal/server/sessions.go), so the transition changes no stored field and
// publishes no SSE event. A client that only refetches on an event never sees it.
// The dashboard stopped polling the moment the stream connected, so a machine
// whose watcher had died stayed `working` on screen for as long as the tab was
// left open, under a green `live` chip.

import assert from "node:assert/strict";
import { test } from "node:test";

const APP = new URL("../../internal/web/static/app.js", import.meta.url);

// A fresh module instance per test: app.js holds its state at module scope, and
// the ESM cache would otherwise hand every test the previous one's timers.
let instance = 0;

// flush lets app.js's pending microtasks (the awaited fetches) settle. Several
// turns, because `start()` awaits a chain of them.
const flush = async (n = 8) => { for (let i = 0; i < n; i++) await new Promise((r) => setImmediate(r)); };

// harness replaces the browser surface app.js touches. Timers are recorded
// rather than scheduled, so a test fires them when it means to and no test waits
// on a real clock.
function harness({ token = "t0k3n", sessions = [], group = null, idle = null, showEnded = null, watcher = null } = {}) {
  const h = {
    now: 1_000_000,
    fetches: [],          // every path app.js requested, in order
    intervals: [],        // {fn, ms}
    timeouts: [],         // {fn, ms}
    aborts: 0,
    stream: null,         // the reader handed to app.js for /api/events
    sessions,             // what GET /api/sessions answers; a test may replace it
  };

  // A permissive DOM stand-in, as in boot.test.mjs — with one addition: an
  // `innerHTML` write is recorded, per element id. That is not a fiction the way
  // a rendered-output assertion would be; it is simply "did app.js redraw".
  h.writes = [];
  h.listeners = new Map();   // "id:event" -> handler, so a test can fire one
  const elements = new Map();
  const element = (id) => {
    const node = new Proxy(function () {}, {
      get: (_t, k) =>
        k === "addEventListener" ? (ev, fn) => h.listeners.set(`${id}:${ev}`, fn)
          : k === "classList" || k === "style" || k === "dataset" ? node
            : k === "hidden" || k === "value" || k === "innerHTML" || k === "textContent" ? ""
              : k === "children" || k === "options" ? []
                : k === Symbol.toPrimitive || k === "toString" ? () => ""
                  : node,
      set: (_t, k, v) => { if (k === "innerHTML") h.writes.push({ id, html: v }); return true; },
      apply: () => node,
      has: () => true,
    });
    return node;
  };
  const byId = (id) => { if (!elements.has(id)) elements.set(id, element(id)); return elements.get(id); };
  globalThis.document = {
    getElementById: byId, querySelectorAll: () => [], createElement: () => byId("<new>"),
    addEventListener: () => {}, documentElement: byId("<root>"), body: byId("<body>"),
  };
  globalThis.window = globalThis;
  globalThis.localStorage = {
    getItem: (k) => (k === "vigie_token" ? token : k === "vigie_group" ? group : k === "vigie_idle_hide" ? idle : k === "vigie_show_ended" ? showEnded : null),
    setItem() {}, removeItem() {},
  };
  globalThis.matchMedia = () => ({ matches: false, addEventListener() {} });

  globalThis.Date.now = () => h.now;
  globalThis.setInterval = (fn, ms) => { h.intervals.push({ fn, ms }); return h.intervals.length; };
  globalThis.clearInterval = () => {};
  globalThis.setTimeout = (fn, ms) => { h.timeouts.push({ fn, ms }); return h.timeouts.length; };
  globalThis.clearTimeout = () => {};

  globalThis.AbortController = class {
    constructor() { this.signal = { aborted: false }; }
    abort() { this.signal.aborted = true; h.aborts++; if (h.stream) h.stream.fail(); }
  };

  // A stream the test drives: `push` delivers bytes, `fail` unblocks a pending
  // read the way an abort does.
  function makeStream() {
    const chunks = [];
    let waiting = null, failed = false;
    const s = {
      push(text) {
        const v = new TextEncoder().encode(text);
        if (waiting) { const w = waiting; waiting = null; w.resolve({ value: v, done: false }); }
        else chunks.push(v);
      },
      fail() { failed = true; if (waiting) { const w = waiting; waiting = null; w.reject(new Error("aborted")); } },
      reader: {
        read() {
          if (chunks.length) return Promise.resolve({ value: chunks.shift(), done: false });
          if (failed) return Promise.reject(new Error("aborted"));
          return new Promise((resolve, reject) => { waiting = { resolve, reject }; });
        },
      },
    };
    return s;
  }

  globalThis.fetch = async (path) => {
    h.fetches.push(path);
    if (path === "/api/events") {
      h.stream = makeStream();
      return { ok: true, status: 200, body: { getReader: () => h.stream.reader } };
    }
    if (path === "/api/watcher") return { ok: true, status: 200, json: async () => (watcher || {}) };
    return { ok: true, status: 200, json: async () => (path === "/api/sessions" ? h.sessions : {}) };
  };

  h.boot = async () => { await import(`${APP}?i=${++instance}`); await flush(); };
  h.count = (path) => h.fetches.filter((p) => p === path).length;
  h.ticker = () => {
    // The periodic refresh: app.js's only 5 s interval. `metaTimer` is 60 s.
    const t = h.intervals.find((i) => i.ms === 5000);
    assert.ok(t, `no 5 s refresh interval was registered — intervals: ${JSON.stringify(h.intervals.map((i) => i.ms))}`);
    return t;
  };
  h.tick = async () => { h.ticker().fn(); await flush(); };
  h.fire = async (id, ev, e) => {
    const fn = h.listeners.get(`${id}:${ev}`);
    assert.ok(fn, `nothing listens for "${ev}" on #${id} — listeners: ${[...h.listeners.keys()]}`);
    fn(e); await flush();
  };
  h.lastTable = () => {
    const w = h.writes.filter((x) => x.id === "tab-sessions");
    return w.length ? w[w.length - 1].html : "";
  };
  h.lastBot = () => {
    const w = h.writes.filter((x) => x.id === "botbar");
    return w.length ? w[w.length - 1].html : "";
  };
  return h;
}

test("the dashboard keeps asking for sessions while the stream is live", async () => {
  const h = harness();
  await h.boot();

  assert.equal(h.count("/api/events"), 1, "the stream must be opened at start");
  const afterBoot = h.count("/api/sessions");
  assert.ok(afterBoot >= 1, "the session list must be loaded at start");

  // The stream is healthy and says nothing, which is the normal case for a quiet
  // fleet. `ended` and `stale` still arrive on time only if the client asks.
  await h.tick();
  assert.equal(h.count("/api/sessions"), afterBoot + 1,
    "a live stream is not a reason to stop asking: the stale→ended cutoff is evaluated server-side at read time and publishes no event (#538)");

  await h.tick();
  assert.equal(h.count("/api/sessions"), afterBoot + 2, "the refresh must be periodic, not once");
});

test("a keep-alive proves the stream is alive without refetching anything", async () => {
  const h = harness();
  await h.boot();
  const before = h.count("/api/sessions");

  // The server's idle heartbeat is a comment frame (internal/server/events.go).
  h.stream.push(": keep-alive\n\n");
  await flush();
  assert.equal(h.count("/api/sessions"), before,
    "a keep-alive carries no news — refetching on it would undo the delta gate of #258");

  // But it is bytes, so the watchdog must count it as life: 29 s later, still short
  // of the limit measured from the beat, nothing is torn down.
  h.now += 29_000;
  await h.tick();
  assert.equal(h.aborts, 0, "a stream that beat 29 s ago is alive");
  assert.equal(h.count("/api/events"), 1, "it must not be reconnected");
});

test("a sessions event refetches immediately, without waiting for the tick", async () => {
  const h = harness();
  await h.boot();
  const before = h.count("/api/sessions");

  h.stream.push("event: sessions\ndata: {}\n\n");
  await flush();
  assert.equal(h.count("/api/sessions"), before + 1, "an event is the whole point of the stream");
});

test("a stream that goes silent is torn down and reopened", async () => {
  const h = harness();
  await h.boot();
  assert.equal(h.count("/api/events"), 1);

  // A suspended machine: the socket never errors, `read()` simply never returns.
  // Without a watchdog the reconnect path below is unreachable — the function
  // guarding it has not returned (#457).
  h.now += 31_000;
  await h.tick();

  assert.equal(h.aborts, 1, "a stream silent past the limit must be aborted, not waited on");
  assert.equal(h.count("/api/events"), 2, "and reopened");
});

test("a silent stream is not condemned before the limit", async () => {
  const h = harness();
  await h.boot();

  h.now += 20_000;
  await h.tick();
  assert.equal(h.aborts, 0, "20 s of quiet is two missed beats, not a death");
  assert.equal(h.count("/api/events"), 1);
});

// The tick asks every 5 s, which is the fix — but redrawing every 5 s is not.
// #tab-sessions contains the scroll container, so an unconditional innerHTML
// write sends a long fleet list back to the top twelve times a minute, and takes
// the operator's text selection and keyboard focus with it. Asking often and
// redrawing rarely are different things.
const FLEET = [
  { id: "a", machine: "m1", status: "working", last_seen_at: "2026-08-16T09:59:00Z", usage: {} },
  { id: "b", machine: "m1", status: "error", api_error_status: 529, last_seen_at: "2026-08-16T09:58:00Z", usage: {} },
];

test("a fleet that has not changed is not redrawn", async () => {
  const h = harness({ sessions: FLEET });
  await h.boot();
  const drawn = () => h.writes.filter((w) => w.id === "tab-sessions").length;
  assert.ok(drawn() >= 1, "the table must be drawn at start");

  const before = drawn();
  await h.tick();
  await h.tick();
  assert.equal(drawn(), before,
    "identical data must not rewrite #tab-sessions — that is the scroll container, the selection and the focus");
});

test("a fleet that changed is redrawn", async () => {
  const h = harness({ sessions: FLEET });
  await h.boot();
  const drawn = () => h.writes.filter((w) => w.id === "tab-sessions").length;
  const before = drawn();

  h.sessions = [{ ...FLEET[0], status: "waiting" }, FLEET[1]];
  await h.tick();
  assert.equal(drawn(), before + 1, "a status change must reach the screen");
  assert.match(h.writes.at(-1).html, /st-waiting/, "and it must be the new status that is drawn");
});

// The guard must not swallow the very transition #538 is about: the row is
// `working` in the payload until the server's read-time cutoff turns it `ended`,
// and that answer arrives only because the tick asked again.
// This test used to assert `/st-ended/` appeared in the table, and passed for the
// wrong reason: it was matching `<span class="cnt st-ended">` in the summary
// strip's status counters, never a row. Ended sessions are hidden by default, so
// no row could ever have carried that class. Deleting the strip in #548 exposed
// it. What the transition actually does, with the default preference, is empty
// the table and raise the hidden count — so that is what is asserted now.
test("the read-time transition to ended reaches the screen", async () => {
  const h = harness({ sessions: FLEET });
  await h.boot();
  assert.match(h.lastTable(), /st-working/, "the fleet starts out working");

  // What the server starts answering once the reports stop (internal/server/sessions.go).
  h.sessions = FLEET.map((s) => ({ ...s, status: "ended" }));
  await h.tick();
  assert.ok(!h.lastTable().includes("st-working"),
    "a dead watcher must stop reading as `working` — no event announces this, only the tick finds it");
  assert.match(h.lastBot(), /hidden<\/span> 2/,
    "and the rows must not vanish silently — the count is what says the fleet is bigger than the table");
});

test("with ended sessions shown, the transition is visible as a row", async () => {
  const h = harness({ sessions: FLEET, showEnded: "1" });
  await h.boot();
  h.sessions = FLEET.map((s) => ({ ...s, status: "ended" }));
  await h.tick();
  assert.match(h.lastTable(), /st-ended/, "the row itself must carry the new status");
});

// #545. The rule itself is cross-checked against the Go implementation in
// dashboard.test.mjs. What is asserted here is the wiring — typing must actually
// reach the table — because that is the half a port gets wrong while every unit
// test stays green.
const FLEET3 = [
  { id: "a", title: "api-gateway", name: "api-gateway", machine: "minet-dev", project_dir: "/h/gateway", project: "gateway", status: "working", usage: {} },
  { id: "b", title: "web-app", name: "web-app", machine: "minet-dev", project_dir: "/h/web", project: "web", status: "idle", usage: {} },
  { id: "c", title: "data-pipe", name: "data-pipe", machine: "beta", project_dir: "/h/pipe", project: "pipe", status: "idle", remote_control: true, usage: {} },
];

test("typing in the filter narrows the table", async () => {
  const h = harness({ sessions: FLEET3 });
  await h.boot();
  assert.match(h.lastTable(), /api-gateway/);
  assert.match(h.lastTable(), /web-app/);

  await h.fire("filter-input", "input", { target: { value: "wapp" } });
  const html = h.lastTable();
  assert.match(html, /web-app/, "a subsequence match must survive the filter");
  assert.ok(!html.includes("api-gateway"), "a session that does not match must leave the table");
  assert.ok(!html.includes("data-pipe"));
});

test("the rc token reaches the table too", async () => {
  const h = harness({ sessions: FLEET3 });
  await h.boot();
  await h.fire("filter-input", "input", { target: { value: "rc" } });
  const html = h.lastTable();
  assert.match(html, /data-pipe/, "the only remote-controlled session");
  assert.ok(!html.includes("api-gateway"));
});

test("a filter that matches nothing says so instead of looking empty", async () => {
  const h = harness({ sessions: FLEET3 });
  await h.boot();
  await h.fire("filter-input", "input", { target: { value: "zzzz" } });
  assert.match(h.lastTable(), /No sessions match the filter/,
    "an empty fleet and a filter that matched nothing must not look the same");
});

test("clearing the filter brings the fleet back", async () => {
  const h = harness({ sessions: FLEET3 });
  await h.boot();
  await h.fire("filter-input", "input", { target: { value: "zzzz" } });
  await h.fire("filter-input", "input", { target: { value: "" } });
  const html = h.lastTable();
  for (const name of ["api-gateway", "web-app", "data-pipe"]) {
    assert.ok(html.includes(name), `${name} did not come back`);
  }
});

// The filter survives the 5 s refresh: the bar lives outside the painted region
// precisely so a repaint cannot drop what the operator typed (#538, #545).
test("the refresh tick does not clear the filter", async () => {
  const h = harness({ sessions: FLEET3 });
  await h.boot();
  await h.fire("filter-input", "input", { target: { value: "wapp" } });
  await h.tick();
  const html = h.lastTable();
  assert.match(html, /web-app/);
  assert.ok(!html.includes("api-gateway"), "the tick repainted the table and lost the filter");
});

// #546. The wiring: picking a mode must reach the table. The rule itself is
// covered in dashboard.test.mjs and pinned against the Go enum in
// internal/tui/group_shared_test.go.
const FLEET_G = [
  { id: "a", title: "api", name: "api", machine: "minet-dev", project_dir: "/h/gateway", project: "gateway", status: "working", usage: { output_tokens: 1000 } },
  { id: "b", title: "web", name: "web", machine: "minet-dev", project_dir: "/h/web", project: "web", status: "idle", usage: { output_tokens: 500 } },
  { id: "c", title: "pipe", name: "pipe", machine: "beta", project_dir: "/h/web", project: "web", status: "idle", usage: { output_tokens: 200 } },
];

test("choosing a group mode redraws the table with headers", async () => {
  const h = harness({ sessions: FLEET_G });
  await h.boot();
  assert.ok(!h.lastTable().includes('class="grouphead"'), "off means no headers at all");

  await h.fire("group-select", "change", { target: { value: "machine" } });
  const html = h.lastTable();
  assert.match(html, /class="grouphead"/, "a group header row must appear");
  assert.match(html, /minet-dev/);
  assert.match(html, /beta/);
  assert.match(html, /\(2 · 1\.5k\)/, "the header carries the count and the combined tokens");
});

test("grouping by project puts two machines under one key", async () => {
  const h = harness({ sessions: FLEET_G });
  await h.boot();
  await h.fire("group-select", "change", { target: { value: "project" } });
  const html = h.lastTable();
  // `web` holds b (minet-dev) and c (beta): one group of two across two machines.
  assert.match(html, /<span class="gk">web<\/span> <span class="gm">\(2 ·/,
    "the project key is the last path segment, so the two roots meet");
});

test("an unknown stored mode degrades to off rather than blanking the table", async () => {
  const h = harness({ sessions: FLEET_G, group: "quantum" });
  await h.boot();
  const html = h.lastTable();
  assert.ok(!html.includes('class="grouphead"'));
  for (const t of ["api", "web", "pipe"]) assert.ok(html.includes(t), `${t} vanished`);
});

test("grouping composes with the filter — the counts follow what is shown", async () => {
  const h = harness({ sessions: FLEET_G });
  await h.boot();
  await h.fire("group-select", "change", { target: { value: "machine" } });
  // `api` reaches only session a. `web` would not do: it matches session c
  // through its *project* name, which is in the haystack — the kind of thing a
  // test written from memory of the fixture gets wrong.
  await h.fire("filter-input", "input", { target: { value: "api" } });
  const html = h.lastTable();
  assert.ok(!html.includes("beta"), "a group with no matching session must not be drawn");
  assert.match(html, /minet-dev<\/span> <span class="gm">\(1 ·/, "the count is of the filtered rows, not the fleet");
});

// #547. The wiring, and the count that goes with it. `hidden N` exists because
// these preferences filter silently: without it the screen claims five sessions
// while the fleet has thirty (sessions-chrome.md § 2, test 3). It counted only
// ended sessions, so switching idle-hiding on would have made it lie.
const AT = Date.parse("2026-08-17T12:00:00Z");
const agoMin = (m) => new Date(AT - m * 60000).toISOString();
const FLEET_I = [
  { id: "a", title: "fresh", machine: "m", status: "working", last_seen_at: agoMin(1), usage: {} },
  { id: "b", title: "quiet", machine: "m", status: "idle", last_seen_at: agoMin(45), usage: {} },
  { id: "c", title: "older", machine: "m", status: "idle", last_seen_at: agoMin(600), usage: {} },
];

test("an idle-hide preference removes the quiet sessions from the table", async () => {
  const h = harness({ sessions: FLEET_I, idle: String(30 * 60000) });
  h.now = AT;
  await h.boot();
  const html = h.lastTable();
  assert.match(html, /fresh/, "a session heard from a minute ago stays");
  assert.ok(!html.includes("quiet"), "45 minutes of silence is past a 30-minute window");
  assert.ok(!html.includes("older"));
});

test("off shows everything, and is the default", async () => {
  const h = harness({ sessions: FLEET_I });
  h.now = AT;
  await h.boot();
  const html = h.lastTable();
  for (const t of ["fresh", "quiet", "older"]) assert.ok(html.includes(t), `${t} was hidden with the preference off`);
});

test("the hidden count includes the idle-hidden, not just the ended", async () => {
  const h = harness({ sessions: FLEET_I, idle: String(30 * 60000) });
  h.now = AT;
  await h.boot();
  // Two of the three are filtered out; the screen must say so, or it claims a
  // fleet of one.
  assert.match(h.lastBot(), /hidden<\/span> 2/,
    "the count still reports only the ended sessions — the screen understates the fleet");
});

test("a session whose timestamp will not parse is kept, not lost", async () => {
  const h = harness({ sessions: [{ id: "x", title: "unparseable", machine: "m", status: "idle", last_seen_at: "soon", usage: {} }], idle: String(60000) });
  h.now = AT;
  await h.boot();
  assert.match(h.lastTable(), /unparseable/, "a row must not disappear over a date that would not parse");
});

// #548, the twin of internal/tui/chrome_test.go's TestTheSummaryRowIsGone.
//
// The strip failed test 1 of sessions-chrome.md § 2 — *it is not already on
// screen* — and that test says nothing about screen size, so the verdict is the
// same in a browser as in a terminal (#544). What the table cannot say by itself
// is what survives.
const FLEET_S = [
  { id: "a", title: "api", machine: "m", status: "working", last_seen_at: "2026-08-17T11:59:00Z", usage: { output_tokens: 1000 } },
  { id: "b", title: "web", machine: "m", status: "ended", last_seen_at: "2026-08-17T11:59:00Z", usage: { output_tokens: 500 } },
];

test("the summary strip is gone from the dashboard", async () => {
  const h = harness({ sessions: FLEET_S });
  await h.boot();
  const html = h.lastTable();
  assert.ok(!html.includes('class="summary"'), "the strip itself");
  assert.ok(!html.includes('class="cnt'), "the status counts — the exact aggregate of the STATUS column");
  assert.ok(!html.includes(">out<"), "the output total");
  assert.ok(!html.includes(">rc<"), "the rc count");
  assert.ok(!html.includes('class="metric'), "and the scopes it mixed at one visual rank");
});

test("the per-session activity sparkline survives — it says what the table cannot", async () => {
  const h = harness({ sessions: [{ ...FLEET_S[0], samples: [1, 5, 3] }] });
  await h.boot();
  assert.match(h.lastTable(), /<svg class="spark"/, "the column keeps its sparkline; only the aggregate went");
});

test("hidden N moves to the bottom bar, and only shows when something is hidden", async () => {
  const h = harness({ sessions: FLEET_S });
  await h.boot();
  assert.match(h.lastBot(), /hidden<\/span> 1/, "one ended session is filtered out of the table");
  // Precise, not the bare word: `aria-hidden="true"` rides on the detail button.
  assert.ok(!h.lastTable().includes('class="hiddenn"'), "and it is no longer above the table");

  // Nothing hidden: the row goes. A permanent zero trains the eye to skip the
  // place where the exception will appear (sessions-chrome.md § 2, test 2).
  const h2 = harness({ sessions: [FLEET_S[0]] });
  await h2.boot();
  assert.ok(!h2.lastBot().includes('class="hiddenn"'), "nothing is hidden, yet the bar says so");
});

// #550. The wiring: the four new columns must actually render, and a saved v1
// layout must survive the rename rather than dropping columns.
const FLEET_C = [{
  id: "a", title: "api", name: "api", user: "nico", machine: "m",
  project_dir: "/h/gateway", project: "gateway",
  status: "working", model: "claude-opus-4-5", model_short: "opus-4-5",
  permission_mode: "plan", mode_label: "plan", mode_detail: "plan — awaiting plan approval",
  context_tokens: 100000, context_pct: 50, last_seen_at: "2026-08-17T11:59:00Z",
  usage: { input_tokens: 10, output_tokens: 5000, cache_read_tokens: 85 },
}];

test("the four columns the dashboard was missing now render", async () => {
  const h = harness({ sessions: FLEET_C });
  await h.boot();
  const html = h.lastTable();
  assert.match(html, />nico</, "user");
  assert.match(html, />50%</, "ctx — the percentage the daemon derived (ADR-0011)");
  assert.match(html, />5\.0k</, "out, on its own rather than only inside the total — with the decimal the terminal keeps (#619)");
  assert.match(html, />plan</, "mode");
});

test("an unknown context reading is a dash, not a zero", async () => {
  const h = harness({ sessions: [{ ...FLEET_C[0], context_tokens: null, context_pct: null }] });
  await h.boot();
  assert.match(h.lastTable(), /class="num faint">-</, "no reading at all must not read as an empty window");
});

test("a layout saved before the rename survives it", async () => {
  // The operator had hidden `branch` and moved `tokens` to the front, under the
  // old key names. Both must still hold after the migration.
  const v1 = JSON.stringify({ order: ["tokens", "name", "activity"], hidden: ["branch"] });
  const store = new Map([["vigie_token", "t0k3n"], ["vigie_columns", v1]]);
  const h = harness({ sessions: FLEET_C });
  globalThis.localStorage = {
    getItem: (k) => (store.has(k) ? store.get(k) : null),
    setItem: (k, v) => store.set(k, v),
    removeItem: (k) => store.delete(k),
  };
  await h.boot();

  const saved = JSON.parse(store.get("vigie_columns_v2"));
  assert.deepEqual(saved.order.slice(0, 3), ["total", "name", "act"], "the renamed keys kept their positions");
  assert.deepEqual(saved.hidden, ["branch"], "and the hidden one stayed hidden");
  assert.equal(store.has("vigie_columns"), false, "the old key is removed, so the remap cannot run twice");
  assert.ok(!h.lastTable().includes("Branch"), "the hidden column is still hidden after the migration");
});

// The dashboard fetched /api/watcher and read only the version string out of it,
// so a machine whose watcher died went on showing frozen statuses with nothing
// saying so — the defect #599 fixed in the terminal, still standing in the
// browser (#623).
const AT_WATCH = Date.parse("2026-08-26T12:00:00Z");
const watcherAt = (offsets) => ({
  machines: Object.fromEntries(Object.entries(offsets).map(
    ([name, ms]) => [name, ms === null ? "" : new Date(AT_WATCH - ms).toISOString().replace(/\.\d+Z$/, "Z")])),
  versions: {},
});

test("a watcher that stopped raises the alarm and is named", async () => {
  const h = harness({ sessions: FLEET_C, watcher: watcherAt({ orion: 60_000, box: 2_000, nova: 2_000 }) });
  h.now = AT_WATCH;
  await h.boot();
  assert.match(h.lastBot(), /1 of 3 not reporting \(orion\)/,
    "the bottom bar must name the machine whose statuses are frozen");
});

test("a healthy fleet says nothing about watchers", async () => {
  const h = harness({ sessions: FLEET_C, watcher: watcherAt({ box: 2_000, nova: 2_000 }) });
  h.now = AT_WATCH;
  await h.boot();
  assert.ok(!h.lastBot().includes("watcher"),
    "a permanent green trains the eye to skip the place where the exception appears");
});

// The reason the rule is not computed by the daemon (ADR-0011's third category,
// #617): the verdict is a function of *now*, so it has to decay here. The server
// is asked nothing between these two assertions — only the clock moves.
test("the alarm appears as time passes, with no new answer from the server", async () => {
  const h = harness({ sessions: FLEET_C, watcher: watcherAt({ orion: 2_000, box: 2_000 }) });
  h.now = AT_WATCH;
  await h.boot();
  assert.ok(!h.lastBot().includes("watcher"), "both watchers are fresh at boot");

  const asked = h.count("/api/watcher");
  h.now += 30_000; // past the 15 s threshold, without refetching anything
  await h.tick();
  assert.equal(h.count("/api/watcher"), asked, "this must not depend on asking the server again");
  assert.match(h.lastBot(), /2 of 2 not reporting \(box, orion\)/,
    "a verdict the daemon had computed would still read as live here");
});
