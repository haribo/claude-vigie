// Tests for the dashboard's pure helpers (internal/web/static/lib.js).
//
// They import the shipped file directly — no copy, no build step — so a test can
// only pass against the code the daemon actually embeds.

import assert from "node:assert/strict";
import { test } from "node:test";
import { readFile } from "node:fs/promises";

import {
  esc, dash, trim, hasCall, detailText, humanTokens, relAge, relResetHint,
  shortModel, projectName, totalTokens, sparkSVG, migrateKeys, fullColOrder, colHidden, rank,
  adoptLegacyKey, ATTENTION, needsAttention, attentionCount, streamIsSilent, SILENCE_MS,
  fuzzyMatch, sessionHaystack, sessionName, shortId, matchesFilter,
  GROUP_MODES, groupKeyOf, groupSessions, IDLE_PRESETS_MS, idleLabel, hiddenByIdle,
} from "../../internal/web/static/lib.js";

// esc is the dashboard's only defence against DOM-based XSS: session titles,
// branches and detail text all come from transcripts, and the operator's token
// sits in localStorage (#161). It is the most important function in the file.
test("esc neutralizes every character that can break out of HTML", () => {
  assert.equal(esc(`<script>alert(1)</script>`),
    "&lt;script&gt;alert(1)&lt;/script&gt;");
  assert.equal(esc(`" onload="x`), "&quot; onload=&quot;x");
  assert.equal(esc(`' onerror='x`), "&#39; onerror=&#39;x");
  // The ampersand must be escaped first, or the other replacements are re-escaped.
  assert.equal(esc("&lt;"), "&amp;lt;");
  assert.equal(esc(null), "");
  assert.equal(esc(undefined), "");
  assert.equal(esc(0), "0");
});

test("dash shows a placeholder only for genuinely absent values", () => {
  assert.equal(dash(""), "-");
  assert.equal(dash(null), "-");
  assert.equal(dash(undefined), "-");
  assert.equal(dash(0), 0, "zero is a value, not an absence");
  assert.equal(dash("x"), "x");
});

test("trim drops a trailing .0 and keeps one decimal otherwise", () => {
  assert.equal(trim(1.0), "1");
  assert.equal(trim(1.25), "1.3");
  assert.equal(trim(10.04), "10");
});

// A call is marked by call_at; the message is optional (ADR-0010).
test("hasCall keys off call_at, not the message", () => {
  assert.equal(hasCall({ call_at: "2026-08-14T10:00:00Z" }), true);
  assert.equal(hasCall({ call_at: "2026-08-14T10:00:00Z", call_message: "" }), true);
  assert.equal(hasCall({ call_message: "done" }), false, "a message without call_at is not a call");
  assert.equal(hasCall({}), false);
  assert.equal(hasCall(null), false);
});

test("detailText lets a call outrank the last tool, and escapes both", () => {
  assert.equal(detailText({ call_at: "t", call_message: "build done" }), "build done");
  assert.equal(detailText({ call_at: "t" }), "called you");
  assert.equal(detailText({ call_at: "t", call_message: "<img src=x onerror=1>" }),
    "&lt;img src=x onerror=1&gt;", "a call message reaches the DOM and must be escaped");
  assert.equal(detailText({ detail: "Bash" }), "Bash");
  assert.equal(detailText({ detail: "<b>" }), "&lt;b&gt;");
  assert.equal(detailText({}), "-");
  assert.equal(detailText({ call_at: "t", detail: "Bash" }), "called you",
    "the call is why the row is pulsing; it takes the cell");
});

test("humanTokens scales to k and M", () => {
  assert.equal(humanTokens(0), "0");
  assert.equal(humanTokens(999), "999");
  assert.equal(humanTokens(1000), "1k");
  assert.equal(humanTokens(1500), "1.5k");
  assert.equal(humanTokens(1_000_000), "1M");
  assert.equal(humanTokens(2_350_000), "2.4M");
  assert.equal(humanTokens(null), "0");
  assert.equal(humanTokens("nonsense"), "0");
});

test("relAge steps through seconds, minutes, hours, days", () => {
  const ago = (s) => new Date(Date.now() - s * 1000).toISOString();
  assert.equal(relAge(ago(5)), "5s");
  assert.equal(relAge(ago(90)), "1m");
  assert.equal(relAge(ago(3 * 3600)), "3h");
  assert.equal(relAge(ago(2 * 86400)), "2d");
  assert.equal(relAge("not a date"), "-");
  assert.equal(relAge(new Date(Date.now() + 60_000).toISOString()), "0s",
    "a future timestamp clamps to zero rather than going negative");
});

test("relResetHint counts down and never shows a past reset", () => {
  const inSec = (s) => new Date(Date.now() + s * 1000).toISOString();
  assert.equal(relResetHint(inSec(-10)), "resets soon");
  assert.match(relResetHint(inSec(30 * 60)), /^resets in \d+m$/);
  assert.match(relResetHint(inSec(5 * 3600)), /^resets in \d+h \d+m$/);
  assert.match(relResetHint(inSec(3 * 86400)), /^resets in \d+d \d+h$/);
  assert.equal(relResetHint("not a date"), "");
});

test("shortModel and projectName reduce identifiers to what fits a column", () => {
  assert.equal(shortModel("claude-opus-4-8"), "opus-4-8");
  assert.equal(shortModel(""), "");
  assert.equal(shortModel(null), "");
  assert.equal(projectName("/home/u/dev/api-gateway"), "api-gateway");
  assert.equal(projectName("/home/u/dev/api-gateway///"), "api-gateway");
  assert.equal(projectName(""), "-");
});

test("totalTokens sums all four buckets and tolerates missing ones", () => {
  assert.equal(totalTokens({ input_tokens: 1, output_tokens: 2, cache_creation_tokens: 3, cache_read_tokens: 4 }), 10);
  assert.equal(totalTokens({}), 0);
});

test("sparkSVG degrades instead of drawing a flat lie", () => {
  assert.match(sparkSVG([]), /faint/, "no data is not a zero line");
  assert.match(sparkSVG(null), /faint/);
  assert.match(sparkSVG([0, 0, 0]), /faint/, "an all-zero series has nothing to show");
  const svg = sparkSVG([1, 5, 3]);
  assert.match(svg, /^<svg /);
  assert.match(svg, /polyline/);
  assert.ok(!svg.includes("NaN"), "coordinates must never render as NaN");
});

// migrateKeys is what keeps a saved layout across a column rename (#393).
test("migrateKeys renames legacy keys and collapses the duplicate", () => {
  assert.deepEqual(migrateKeys(["doing"]), ["detail"]);
  assert.deepEqual(migrateKeys(["doing", "detail"]), ["detail"],
    "renaming must not leave the column listed twice");
  assert.deepEqual(migrateKeys(["name", "doing", "status"]), ["name", "detail", "status"]);
  assert.deepEqual(migrateKeys([]), []);
});

const KEYS = ["name", "status", "detail", "tokens"];
const MANDATORY = new Set(["name", "status"]);

test("fullColOrder keeps the saved order, drops unknowns, appends new columns", () => {
  assert.deepEqual(fullColOrder(["detail", "name"], KEYS), ["detail", "name", "status", "tokens"]);
  assert.deepEqual(fullColOrder(["gone", "name"], KEYS), ["name", "status", "detail", "tokens"],
    "a column removed from the build must not linger in the layout");
  assert.deepEqual(fullColOrder([], KEYS), KEYS);
  assert.deepEqual(fullColOrder(null, KEYS), KEYS);
  assert.deepEqual(fullColOrder(["name", "name"], KEYS), KEYS, "duplicates collapse");
});

test("colHidden can never hide a mandatory column", () => {
  assert.equal(colHidden(["detail"], "detail", MANDATORY), true);
  assert.equal(colHidden(["name"], "name", MANDATORY), false, "name is mandatory");
  assert.equal(colHidden(["status"], "status", MANDATORY), false);
  assert.equal(colHidden([], "detail", MANDATORY), false);
});

// The dashboard is a module graph now: a name app.js imports that lib.js does not
// export kills the whole page at load, silently as far as the daemon is concerned
// (the file still serves 200). This checks the graph without a browser.
test("lib.js exports every name app.js imports from it", async () => {
  const src = await readFile(new URL("../../internal/web/static/app.js", import.meta.url), "utf8");
  const m = src.match(/import\s*\{([^}]*)\}\s*from\s*"\.\/lib\.js"/);
  assert.ok(m, "app.js no longer imports from ./lib.js — has the wiring changed?");

  const imported = m[1].split(",").map((s) => s.trim()).filter(Boolean);
  assert.ok(imported.length > 0, "the import list is empty");

  const lib = await import("../../internal/web/static/lib.js");
  const missing = imported.filter((name) => !(name in lib));
  assert.deepEqual(missing, [], `app.js imports names lib.js does not export: ${missing}`);
});

// The status sort. `RANK` used to be an object holding eight of the nine
// statuses, so `RANK["compacting"]` was undefined and the comparator returned
// NaN — which does not order badly, it stops ordering: an `ended` session came
// out first (#464).
test("rank places every status, most active first", () => {
  const order = ["stalled", "working", "thinking", "compacting", "waiting", "idle", "error", "stale", "ended"];
  order.forEach((s, i) => assert.equal(rank(s), i, `${s} is out of place`));
});

test("an unknown status sorts last, never first", () => {
  assert.ok(rank("quantum") > rank("ended"),
    "a status this build has never heard of must not head the table");
  assert.ok(Number.isFinite(rank("quantum")), "an unknown status must still compare");
});

test("the comparator orders instead of returning NaN", () => {
  const cmp = (a, b) => rank(a.status) - rank(b.status);
  const rows = ["ended", "compacting", "working", "stale", "quantum", "stalled"].map((status) => ({ status }));
  assert.deepEqual([...rows].sort(cmp).map((r) => r.status),
    ["stalled", "working", "compacting", "stale", "ended", "quantum"]);
  for (const a of rows) {
    for (const b of rows) {
      assert.ok(!Number.isNaN(cmp(a, b)), `cmp(${a.status}, ${b.status}) is NaN`);
    }
  }
});

// A localStorage stand-in: the same three methods the dashboard uses.
function fakeStorage(initial = {}) {
  const m = new Map(Object.entries(initial));
  return {
    getItem: (k) => (m.has(k) ? m.get(k) : null),
    setItem: (k, v) => m.set(k, v),
    removeItem: (k) => m.delete(k),
    keys: () => [...m.keys()].sort(),
  };
}

// #478. The dashboard's storage keys carried the old brand and hold live state:
// renaming them without carrying the values over signs every operator out and
// drops their column layout.
test("a value saved under the old key survives the rename", () => {
  const s = fakeStorage({ cf_token: "tok-123" });
  assert.equal(adoptLegacyKey(s, "cf_token", "vigie_token"), "tok-123",
    "the operator was signed out by a rename");
  assert.equal(s.getItem("vigie_token"), "tok-123", "the value was not carried over");
  assert.deepEqual(s.keys(), ["vigie_token"], "the old key must not linger");
});

test("a value written since the upgrade wins over a stale leftover", () => {
  const s = fakeStorage({ cf_token: "old", vigie_token: "new" });
  assert.equal(adoptLegacyKey(s, "cf_token", "vigie_token"), "new",
    "a leftover old key rolled the current value back");
  assert.deepEqual(s.keys(), ["vigie_token"]);
});

test("nothing stored stays nothing stored", () => {
  const s = fakeStorage();
  assert.equal(adoptLegacyKey(s, "cf_token", "vigie_token"), null);
  assert.deepEqual(s.keys(), [], "a fresh browser must not gain an empty key");
});

test("the migration is idempotent — the dashboard calls it on every layout read", () => {
  const s = fakeStorage({ cf_columns: '{"order":["name"],"hidden":[]}' });
  const first = adoptLegacyKey(s, "cf_columns", "vigie_columns");
  assert.equal(adoptLegacyKey(s, "cf_columns", "vigie_columns"), first);
  assert.deepEqual(s.keys(), ["vigie_columns"]);
});

// An empty string is a real stored value (a cleared token), not an absent key —
// and `|| null` in the wrong place would turn one into the other.
test("an empty stored value is carried over, not dropped", () => {
  const s = fakeStorage({ cf_token: "" });
  assert.equal(adoptLegacyKey(s, "cf_token", "vigie_token"), "");
  assert.deepEqual(s.keys(), ["vigie_token"]);
});

// #538. Three clients decide when to interrupt the operator, and the dashboard
// was the one that decided on its own: row highlighting was
// `call || waiting || stalled`, so an `error` session carried no mark, and the
// tab badge counted `waiting` alone. #466 had already settled this for the GNOME
// indicator — one list, consulted rather than reimplemented — and left the
// dashboard out. `TestDashboardSharesTheAttentionSet` pins the list to
// internal/status.Attention; these pin what the dashboard does with it.
test("needsAttention covers every attention status and a raised call", () => {
  for (const status of ATTENTION) {
    assert.equal(needsAttention({ status }), true, `${status} must call the operator`);
  }
  assert.equal(needsAttention({ status: "error" }), true,
    "an API error is an attention status — this is the one the dashboard dropped");
  // A call rides alongside a status rather than being one (ADR-0010).
  assert.equal(needsAttention({ status: "working", call_at: "2026-08-16T10:00:00Z" }), true);
  assert.equal(needsAttention({ status: "idle", call_at: "2026-08-16T10:00:00Z" }), true);
});

test("needsAttention leaves a session that needs nobody alone", () => {
  for (const status of ["working", "thinking", "compacting", "idle", "stale", "ended"]) {
    assert.equal(needsAttention({ status }), false, `${status} must not interrupt`);
  }
  assert.equal(needsAttention({ status: "waiting", call_at: "" }), true, "an empty call is not a call, the status still is");
  assert.equal(needsAttention(null), false);
  assert.equal(needsAttention(undefined), false);
});

test("attentionCount counts every reason to interrupt, not just waiting", () => {
  const sessions = [
    { id: "a", status: "waiting" },
    { id: "b", status: "error" },
    { id: "c", status: "stalled" },
    { id: "d", status: "working", call_at: "2026-08-16T10:00:00Z" },
    { id: "e", status: "working" },
    { id: "f", status: "idle" },
  ];
  assert.equal(attentionCount(sessions), 4, "the badge must not count `waiting` alone");
  assert.equal(attentionCount([]), 0);
  assert.equal(attentionCount(null), 0);
});

// #538/#457. A silent stream is indistinguishable from a dead one. On a machine
// that suspends, the connection dies with no FIN and no RST: the socket stays
// open as far as the page is concerned and `read()` blocks for minutes, so the
// reconnect path never runs. The server sends a keep-alive comment every 10 s
// (internal/server/events.go), which makes silence measurable.
test("streamIsSilent waits three missed beats before calling a stream dead", () => {
  const t0 = 1_000_000;
  assert.equal(SILENCE_MS, 30000, "three missed 10 s beats, same window the TUI uses");
  assert.equal(streamIsSilent(t0, t0), false, "a stream heard from just now is alive");
  assert.equal(streamIsSilent(t0, t0 + 10_000), false, "one beat late is not dead");
  assert.equal(streamIsSilent(t0, t0 + SILENCE_MS), false, "the limit itself is not past it");
  assert.equal(streamIsSilent(t0, t0 + SILENCE_MS + 1), true, "past the limit the stream is dead");
});

test("streamIsSilent never condemns a stream it has never heard from", () => {
  // Nothing heard yet means the connection is still being established, not that
  // it died — condemning it here would abort every connect before it opened.
  assert.equal(streamIsSilent(0, 1_000_000), false);
  assert.equal(streamIsSilent(null, 1_000_000), false);
  assert.equal(streamIsSilent(undefined, 1_000_000), false);
});

// #545. The dashboard's filter is a hand port of `fuzzyMatch` from
// internal/tui/model.go. A rule copied per consumer is what #421, #422 and #466
// were, so the copy is not trusted: this reads the same fixture that
// internal/tui/filter_shared_test.go reads, and the two must agree case for case.
test("fuzzyMatch agrees with the shared fixture the Go side reads", async () => {
  const raw = await readFile(new URL("../fixtures/fuzzy-cases.json", import.meta.url), "utf8");
  const { cases } = JSON.parse(raw);
  assert.ok(cases.length > 0, "the shared fixture has no cases — the extraction is broken, not the code");
  for (const c of cases) {
    assert.equal(fuzzyMatch(c.pattern, c.text), c.want,
      `fuzzyMatch(${JSON.stringify(c.pattern)}, ${JSON.stringify(c.text)}) — ${c.why}`);
  }
});

// Field order and the single spaces are part of the rule: a pattern may run from
// the end of one field into the start of the next. The Go side asserts the same
// example in TestSessionHaystackShape.
test("the haystack has the same shape as the TUI's", () => {
  const s = { title: "api-gateway", machine: "minet-dev", project_dir: "/home/nico/gateway", git_branch: "main", status: "working" };
  assert.equal(sessionHaystack(s), "api-gateway minet-dev gateway main working");
});

test("an untitled session is named, and searchable, by its short id", () => {
  assert.equal(shortId("abcdefghij-the-rest"), "abcdefgh");
  assert.equal(shortId("short"), "short");
  assert.equal(sessionName({ title: "", id: "abcdefghij-the-rest" }), "abcdefgh");
  assert.equal(sessionName({ title: "named", id: "abcdefghij" }), "named");
  const s = { id: "abcdefghij-the-rest", machine: "m", status: "idle" };
  assert.equal(fuzzyMatch("abcdefghij", sessionHaystack(s)), false,
    "the ninth character of the id must not be searchable — the name shows eight");
});

test("rc is a token as a whole pattern, and ordinary text otherwise", () => {
  const on = { id: "a", machine: "m", status: "idle", remote_control: true };
  const off = { id: "b", machine: "m", status: "idle", remote_control: false };
  assert.equal(matchesFilter(on, "rc"), true);
  assert.equal(matchesFilter(off, "rc"), false, "the token selects, it does not text-match");
  assert.equal(matchesFilter(on, "RC"), true, "the token is case-insensitive");
  // As part of a longer pattern it is not the token: it must match the text.
  const src = { id: "c", machine: "src-tool", status: "idle", remote_control: false };
  assert.equal(matchesFilter(src, "rct"), true, "`rct` is a subsequence of `src-tool`, not the token");
  assert.equal(matchesFilter(off, "rct"), false);
});

test("an empty filter selects everything", () => {
  const s = { id: "a", machine: "m", status: "idle" };
  assert.equal(matchesFilter(s, ""), true);
  assert.equal(matchesFilter(s, null), true);
  assert.equal(matchesFilter(s, undefined), true);
});

// #546. Grouping. The mode names are pinned against the Go enum by
// TestDashboardSharesTheGroupModes; what is asserted here is what grouping does.
const G = (id, machine, dir, out) => ({ id, machine, project_dir: dir, status: "idle", usage: { output_tokens: out } });

test("off returns the list untouched, in one group with no key", () => {
  const list = [G("a", "m1", "/h/x", 1), G("b", "m2", "/h/y", 2)];
  const groups = groupSessions(list, "off");
  assert.equal(groups.length, 1);
  assert.equal(groups[0].key, null, "a null key is how the renderer knows to draw no header");
  assert.deepEqual(groups[0].sessions.map((s) => s.id), ["a", "b"]);
});

test("the project key is the last path segment, so two roots meet in one group", () => {
  assert.equal(groupKeyOf({ project_dir: "/home/nico/dev/api-gateway" }, "project"), "api-gateway");
  assert.equal(groupKeyOf({ project_dir: "/srv/build/api-gateway" }, "project"), "api-gateway");
  assert.equal(groupKeyOf({ machine: "minet-dev" }, "machine"), "minet-dev");
});

test("grouping keeps the operator's sort inside each group", () => {
  // Already sorted by id descending; the re-sort by group must not disturb that.
  const list = [G("a3", "beta", "/h/x", 0), G("a2", "alpha", "/h/x", 0), G("a1", "beta", "/h/x", 0)];
  const groups = groupSessions(list, "machine");
  assert.deepEqual(groups.map((g) => g.key), ["alpha", "beta"], "groups come out in key order");
  assert.deepEqual(groups[1].sessions.map((s) => s.id), ["a3", "a1"],
    "inside a group the incoming order survives — a single non-stable sort would lose it");
});

test("each group carries its count and its combined tokens", () => {
  const list = [
    { id: "a", machine: "m", status: "idle", usage: { input_tokens: 10, output_tokens: 5 } },
    { id: "b", machine: "m", status: "idle", usage: { cache_read_tokens: 85 } },
  ];
  const [g] = groupSessions(list, "machine");
  assert.equal(g.count, 2);
  assert.equal(g.tokens, 100, "all four buckets, not output alone — the TUI's subtotal is totalTokens");
});

test("a session with no machine still groups, under the empty key", () => {
  const groups = groupSessions([{ id: "a", status: "idle", usage: {} }], "machine");
  assert.equal(groups.length, 1);
  assert.equal(groups[0].key, "", "the renderer dashes it, the way the TUI's orDash does");
});

test("GROUP_MODES is the vocabulary, off first", () => {
  assert.deepEqual(GROUP_MODES, ["off", "machine", "project"]);
});

// #547. Hiding a session that has gone quiet. Three details of the TUI's rule
// (internal/tui/prefs.go) are easy to get wrong, and each is a separate test
// because each fails differently.
const NOW = Date.parse("2026-08-17T12:00:00Z");
const seenAgo = (min) => ({ id: "s", status: "idle", last_seen_at: new Date(NOW - min * 60000).toISOString() });

test("hiddenByIdle hides a session unheard from for longer than the window", () => {
  assert.equal(hiddenByIdle(seenAgo(31), 30 * 60000, NOW), true);
  assert.equal(hiddenByIdle(seenAgo(29), 30 * 60000, NOW), false);
  assert.equal(hiddenByIdle(seenAgo(30), 30 * 60000, NOW), false,
    "the boundary itself is still visible — the TUI compares with <=");
});

test("off means never, and it is the default", () => {
  assert.equal(hiddenByIdle(seenAgo(10000), 0, NOW), false);
  assert.equal(hiddenByIdle(seenAgo(10000), null, NOW), false);
  assert.equal(IDLE_PRESETS_MS[0], 0);
});

test("the clock is last_seen_at, not the status", () => {
  // A `working` session whose reports stopped is hidden too. Deliberate: it is
  // the same "nothing is happening here" signal, and the TUI does the same.
  const working = { ...seenAgo(90), status: "working" };
  assert.equal(hiddenByIdle(working, 60 * 60000, NOW), true);
});

test("an unreadable timestamp keeps the session visible", () => {
  // Losing a row over a date that would not parse is worse than one row too many.
  assert.equal(hiddenByIdle({ id: "s", last_seen_at: "not a date" }, 60000, NOW), false);
  assert.equal(hiddenByIdle({ id: "s" }, 60000, NOW), false);
  assert.equal(hiddenByIdle({ id: "s", last_seen_at: "" }, 60000, NOW), false);
});

test("the presets and their labels match what the TUI offers", () => {
  assert.deepEqual(IDLE_PRESETS_MS, [0, 900000, 1800000, 3600000, 10800000, 21600000]);
  assert.deepEqual(IDLE_PRESETS_MS.map(idleLabel), ["off (never)", "15m", "30m", "1h", "3h", "6h"]);
});
