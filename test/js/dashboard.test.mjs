// Tests for the dashboard's pure helpers (internal/web/static/lib.js).
//
// They import the shipped file directly — no copy, no build step — so a test can
// only pass against the code the daemon actually embeds.

import assert from "node:assert/strict";
import { test } from "node:test";
import { readFile } from "node:fs/promises";

import {
  esc, dash, trim, hasCall, detailText, humanTokens, relAge, relResetHint,
  totalTokens, sparkSVG, migrateKeys, fullColOrder, colHidden, rank,
  adoptLegacyKey, needsAttention, attentionCount, streamIsSilent, SILENCE_MS,
  readWatcher, fleetAlarm, fleetAlarmDetail, watcherCell,
  fuzzyMatch, sessionHaystack, matchesFilter,
  GROUP_MODES, groupKeyOf, groupSessions, IDLE_PRESETS_MS, idleLabel, hiddenByIdle,
  contextCell, contextKnown, contextPct, migrateV1Columns, V1_COLUMN_KEYS, STATUSES,
  SORT_COMPARATORS, DEFAULT_SORT, boardState, emptyMessage,
  attentionIds, enteredAttention, nextAttention,
} from "../../internal/web/static/lib.js";

// Every shared case list is read the same way; the interesting part is what each
// test asserts, not the four lines that open the file (#619).
const fixture = async (name) =>
  JSON.parse(await readFile(new URL(`../fixtures/${name}`, import.meta.url), "utf8"));

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

// What the cell shows and in what order — a call, then an API error, then the
// activity — is the daemon's since #618 and is proved in
// internal/server/naming_test.go. What is owed here is that the derived text is
// rendered rather than recomputed, and that it is escaped: it carries a session
// title and a call message, and this one reaches the DOM.
test("detailText renders what the daemon derived, and escapes it", () => {
  assert.equal(detailText({ detail_text: "build done" }), "build done");
  assert.equal(detailText({ detail_text: "<img src=x onerror=1>" }),
    "&lt;img src=x onerror=1&gt;");
  assert.equal(detailText({ detail_text: "-" }), "-");
  assert.equal(detailText({}), "-", "a view with no derived text still renders a dash, never `undefined`");
  assert.equal(detailText({ detail_text: "529 Overloaded", detail: "Bash" }), "529 Overloaded",
    "the raw detail is beside it and must not be preferred");
});

// The scale itself is in test/fixtures/format-cases.json now, read by the Go side
// too (#619) — this list used to assert `humanTokens(1000) === "1k"` while the Go
// list asserted "1.0k", each green, neither containing the case that separated
// them. What stays here is what the fixture cannot carry: a value that is not a
// number at all, which only this side can be handed.
test("humanTokens survives what is not a number", () => {
  assert.equal(humanTokens(null), "0");
  assert.equal(humanTokens("nonsense"), "0");
  assert.equal(humanTokens(undefined), "0");
});

test("relResetHint counts down and never shows a past reset", () => {
  const inSec = (s) => new Date(Date.now() + s * 1000).toISOString();
  assert.equal(relResetHint(inSec(-10)), "resets soon");
  assert.match(relResetHint(inSec(30 * 60)), /^resets in \d+m$/);
  assert.match(relResetHint(inSec(5 * 3600)), /^resets in \d+h \d+m$/);
  assert.match(relResetHint(inSec(3 * 86400)), /^resets in \d+d \d+h$/);
  assert.equal(relResetHint("not a date"), "");
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
test("rank reads the rank the daemon sent", () => {
  // Which status sorts where is no longer decided here (ADR-0011, #617); it is
  // proved once, in Go, where it is produced. What is left to get wrong on this
  // side is failing to read the answer.
  assert.equal(rank({ status: "stalled", rank: 0 }), 0);
  assert.equal(rank({ status: "ended", rank: 8 }), 8);
});

test("a session with no rank sorts last, never first", () => {
  assert.ok(rank({ status: "quantum" }) > rank({ status: "ended", rank: 8 }),
    "a session this build cannot place must not head the table");
  assert.ok(Number.isFinite(rank({ status: "quantum" })), "an unrankable session must still compare");
  assert.ok(Number.isFinite(rank(null)), "so must a missing one");
});

test("the comparator orders instead of returning NaN", () => {
  // #464: four statuses nobody had ranked produced a NaN comparator, and a NaN
  // comparator does not sort badly — it stops sorting. The rank arrives from the
  // daemon now, and a row arriving without one must still compare.
  const cmp = (a, b) => rank(a) - rank(b);
  const ranks = { stalled: 0, working: 1, compacting: 3, stale: 7, ended: 8 };
  const rows = ["ended", "compacting", "working", "stale", "quantum", "stalled"]
    .map((status) => (status in ranks ? { status, rank: ranks[status] } : { status }));
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
test("needsAttention honours the daemon's verdict and adds a raised call", () => {
  // Which statuses are blocking is no longer this file's business: the daemon
  // decides and sends `attention` (ADR-0011, #617), so there is no second list to
  // drop `error` from, which is what #538 was. What is still the dashboard's own
  // is the combination — a call rides alongside a status rather than being one
  // (ADR-0010), so both have to be looked at.
  assert.equal(needsAttention({ status: "waiting", attention: true }), true);
  assert.equal(needsAttention({ status: "working", attention: false }), false);
  assert.equal(needsAttention({ status: "working", call_at: "2026-08-16T10:00:00Z" }), true);
  assert.equal(needsAttention({ status: "idle", call_at: "2026-08-16T10:00:00Z" }), true);
});

test("needsAttention leaves a session that needs nobody alone", () => {
  for (const status of ["working", "thinking", "compacting", "idle", "stale", "ended"]) {
    assert.equal(needsAttention({ status, attention: false }), false, `${status} must not interrupt`);
  }
  assert.equal(needsAttention({ status: "waiting", attention: true, call_at: "" }), true,
    "an empty call is not a call, the status still is");
  assert.equal(needsAttention(null), false);
  assert.equal(needsAttention(undefined), false);
});

test("attentionCount counts every reason to interrupt, not just waiting", () => {
  const sessions = [
    { id: "a", status: "waiting", attention: true },
    { id: "b", status: "error", attention: true },
    { id: "c", status: "stalled", attention: true },
    { id: "d", status: "working", attention: false, call_at: "2026-08-16T10:00:00Z" },
    { id: "e", status: "working", attention: false },
    { id: "f", status: "idle", attention: false },
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
  const { cases } = await fixture("fuzzy-cases.json");
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
  const s = { title: "api-gateway", name: "api-gateway", machine: "orion-dev", project: "gateway", git_branch: "main", status: "working" };
  assert.equal(sessionHaystack(s), "api-gateway orion-dev gateway main working");
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
const G = (id, machine, dir, out) => ({ id, machine, project_dir: dir, project: dir.split("/").pop(),
  status: "idle", usage: { output_tokens: out } });

test("off returns the list untouched, in one group with no key", () => {
  const list = [G("a", "m1", "/h/x", 1), G("b", "m2", "/h/y", 2)];
  const groups = groupSessions(list, "off");
  assert.equal(groups.length, 1);
  assert.equal(groups[0].key, null, "a null key is how the renderer knows to draw no header");
  assert.deepEqual(groups[0].sessions.map((s) => s.id), ["a", "b"]);
});

// The last-path-segment rule is the daemon's (#618) and is proved there; what
// this asserts is that two roots reduced to the same `project` meet in one group.
test("the project key is what the daemon derived, so two roots meet in one group", () => {
  assert.equal(groupKeyOf({ project_dir: "/home/ada/dev/api-gateway", project: "api-gateway" }, "project"), "api-gateway");
  assert.equal(groupKeyOf({ project_dir: "/srv/build/api-gateway", project: "api-gateway" }, "project"), "api-gateway");
  assert.equal(groupKeyOf({ machine: "orion-dev" }, "machine"), "orion-dev");
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

// #550. CTX and MODE are two more rules copied out of Go. Neither copy is
// trusted: this reads the same fixture internal/tui/column_shared_test.go reads.
test("contextCell agrees with the shared fixture the Go side reads", async () => {
  const { context } = await fixture("column-cases.json");
  assert.ok(context.length > 0, "the fixture has no context cases");
  for (const c of context) {
    // The daemon derives `pct` from the model and the reading (ADR-0011); this
    // side is asked only what it renders from the answer.
    const s = { model: c.model, context_tokens: c.tokens, context_pct: c.pct };
    assert.equal(contextCell(s), c.want, `pct=${c.pct} — ${c.why}`);
  }
});

test("unknown and known-to-be-zero are different states", () => {
  // The daemon returns a nil pointer for the first and a 0 for the second, on
  // purpose (#367). Collapsing them here would rebuild the defect it fixed.
  assert.equal(contextKnown({ model: "claude-opus-4-8" }), false);
  assert.equal(contextKnown({ model: "claude-opus-4-8", context_tokens: null, context_pct: null }), false);
  assert.equal(contextKnown({ model: "claude-opus-4-8", context_tokens: 0, context_pct: 0 }), true);
  assert.equal(contextCell({ model: "claude-opus-4-8" }), "-");
  assert.equal(contextCell({ model: "claude-opus-4-8", context_tokens: 0, context_pct: 0 }), "0%");
  assert.equal(contextPct({ model: "claude-opus-4-8" }), 0, "an unknown reading still sorts as zero");
  // Half an invariant is not the invariant: one field without the other renders a
  // dash rather than "undefined%".
  assert.equal(contextCell({ context_tokens: 100000 }), "-");
});

// #550. A saved layout is live state: renaming four keys without carrying it over
// drops columns out of the operator's order, and a hidden one silently reappears.
test("a v1 layout is remapped, once", () => {
  assert.deepEqual(migrateV1Columns(["name", "project", "tokens", "activity", "act"]),
    ["name", "dir", "total", "act", "open"]);
  assert.deepEqual(migrateV1Columns(["doing"]), ["detail"], "the #393 rename is folded in");
  assert.deepEqual(migrateV1Columns([]), []);
  assert.deepEqual(migrateV1Columns(null), []);
});

test("the v1 remap is deliberately not idempotent, which is why it runs once", () => {
  // `activity` → `act` and `act` → `open` in the same pass. Applying it twice
  // turns the freshly-migrated sparkline into the detail button, which is exactly
  // why it must never be merged into LEGACY_COLS (applied on every read).
  const once = migrateV1Columns(["activity"]);
  assert.deepEqual(once, ["act"]);
  assert.deepEqual(migrateV1Columns(once), ["open"],
    "a second pass corrupts it — the storage key bump is what prevents that, not the map");
  assert.equal(V1_COLUMN_KEYS.act, "open");
});

test("the remap collapses a duplicate rather than listing a column twice", () => {
  assert.deepEqual(migrateV1Columns(["tokens", "total"]), ["total"]);
});

// The watcher verdict is duplicated on purpose — it is a function of *now*, so it
// decays between fetches and must be derived where it is displayed (ADR-0011's
// third category, #617). Duplicated, not unchecked: this reads the same case list
// internal/tui/watcher_shared_test.go reads, and the two must agree case for
// case, including the exact alarm text (#623).
test("readWatcher agrees with the shared fixture the Go side reads", async () => {
  const { verdict } = await fixture("watcher-cases.json");
  assert.ok(verdict.length > 0, "the fixture has no verdict cases");
  for (const c of verdict) {
    assert.equal(readWatcher(c.seen, Date.parse(c.now)), c.want, `seen=${c.seen} — ${c.why}`);
  }
});

test("the fleet alarm agrees with the shared fixture, text included", async () => {
  const { fleet } = await fixture("watcher-cases.json");
  assert.ok(fleet.length > 0, "the fixture has no fleet cases");
  for (const c of fleet) {
    const r = fleetAlarm(c.machines, Date.parse(c.now));
    assert.equal(r.alarm, c.alarm, `${JSON.stringify(c.machines)} — ${c.why}`);
    const detail = r.alarm ? fleetAlarmDetail(r.known, r.silent, r.unreadable) : "reporting";
    assert.equal(detail, c.detail, `${JSON.stringify(c.machines)} — ${c.why}`);
  }
});

test("a machine card names which failure it is showing", () => {
  // The two alarms send the operator to different places: a watcher that stopped
  // is on that machine, a heartbeat that will not parse is on this side.
  assert.deepEqual(watcherCell(readWatcher("2026-08-26T12:00:00Z", Date.parse("2026-08-26T12:00:02Z"))),
    { cls: "w-ok", text: "live" });
  assert.deepEqual(watcherCell(readWatcher("2026-08-26T11:00:00Z", Date.parse("2026-08-26T12:00:02Z"))),
    { cls: "w-bad", text: "none" });
  assert.deepEqual(watcherCell(readWatcher("", Date.parse("2026-08-26T12:00:02Z"))),
    { cls: "w-bad", text: "none" });
  assert.deepEqual(watcherCell(readWatcher("not-a-time", Date.parse("2026-08-26T12:00:02Z"))),
    { cls: "w-bad", text: "time?" });
});

// ── The shared case lists of ADR-0011's fourth family (#619) ─────────────────
//
// Grouping, idle hiding and the two figures each client formats are duplicated on
// purpose: they are functions of what the operator typed or chose, and an age is a
// function of *now*. Duplicated, not unchecked — the Go suite reads these same
// files and both must answer every case identically.

test("token formatting agrees with the shared fixture", async () => {
  const { tokens } = await fixture("format-cases.json");
  assert.ok(tokens.length > 0, "the shared fixture has no token cases");
  for (const c of tokens) assert.equal(humanTokens(c.n), c.want, `${c.n} — ${c.why}`);
});

test("relative ages agree with the shared fixture", async () => {
  const { age } = await fixture("format-cases.json");
  assert.ok(age.length > 0, "the shared fixture has no age cases");
  const realNow = Date.now;
  try {
    for (const c of age) {
      Date.now = () => Date.parse(c.now);
      assert.equal(relAge(c.seen), c.want, `${c.seen || "(empty)"} — ${c.why}`);
    }
  } finally {
    Date.now = realNow;
  }
});

test("the group modes agree with the shared fixture", async () => {
  const { modes } = await fixture("group-cases.json");
  assert.deepEqual(GROUP_MODES, modes,
    "a mode renamed on one client and not the other resets an operator's saved grouping");
});

test("group keys agree with the shared fixture", async () => {
  const { keys } = await fixture("group-cases.json");
  assert.ok(keys.length > 0, "the shared fixture has no key cases");
  for (const c of keys) {
    assert.equal(groupKeyOf({ machine: c.machine, project: c.project }, c.mode), c.want, c.why);
  }
});

test("the idle presets agree with the shared fixture", async () => {
  const { presets_seconds } = await fixture("idle-cases.json");
  assert.deepEqual(IDLE_PRESETS_MS, presets_seconds.map((s) => s * 1000),
    "a step offered on one client and not the other leaves a stored threshold the other cannot represent");
});

test("idle hiding agrees with the shared fixture", async () => {
  const { cases } = await fixture("idle-cases.json");
  assert.ok(cases.length > 0, "the shared fixture has no idle cases");
  for (const c of cases) {
    const s = { last_seen_at: c.seen, status: c.status || "idle" };
    const got = hiddenByIdle(s, c.after_seconds * 1000, Date.parse(c.now));
    assert.equal(got, c.hidden, `${c.seen || "(empty)"} after ${c.after_seconds}s — ${c.why}`);
  }
});

test("the dashboard's status vocabulary agrees with the shared fixture", async () => {
  const { order } = await fixture("status-vocabulary.json");
  assert.deepEqual(STATUSES, order,
    "a status the dashboard does not know is styled as `idle` — displayed as something it is not");
});

// #645. What the table opens on, and the order each key produces. The Go suite
// reads the same list: three columns had drifted apart because every shared list
// until now pinned a *rule* and none pinned a starting state.
test("the table opens on the key the shared fixture names", async () => {
  const { default: def } = await fixture("sort-cases.json");
  assert.equal(DEFAULT_SORT.key, def.key);
  assert.equal(DEFAULT_SORT.dir, 1, "the fixture's orders are the comparator's own, unreversed");
});

test("every sort key agrees with the shared fixture", async () => {
  const f = await fixture("sort-cases.json");
  assert.ok(f.cases.length > 0 && f.fleet.length > 1, "the shared fixture is missing a section");
  const realNow = Date.now;
  Date.now = () => Date.parse(f.now);
  try {
    for (const c of f.cases) {
      const cmp = SORT_COMPARATORS[c.key];
      assert.ok(cmp, `no comparator for ${c.key}`);
      const got = [...f.fleet].sort((a, b) => cmp(a, b) * 1).map((s) => s.id);
      assert.deepEqual(got, c.want, `${c.key} — ${c.why}`);
    }
  } finally {
    Date.now = realNow;
  }
});

// `out` and `ctx` are columns the browser sorts and the terminal does not, so the
// fixture has nothing to compare them with. They follow the sense of the numeric
// columns beside them: what is worth the glance comes first.
test("the browser-only numeric columns sort like the ones beside them", () => {
  const small = { id: "small", usage: { output_tokens: 1 }, context_pct: 10, context_tokens: 1 };
  const big = { id: "big", usage: { output_tokens: 9000 }, context_pct: 90, context_tokens: 9 };
  for (const key of ["out", "ctx"]) {
    const got = [small, big].sort((a, b) => SORT_COMPARATORS[key](a, b)).map((s) => s.id);
    assert.deepEqual(got, ["big", "small"], `${key} must open on the notable one`);
  }
});

// #673. The dashboard used to discard every rejection from `loadSessions` and
// `start`, so two different failures wore the face of a healthy, quiet fleet.
// These are the two decisions that were missing, kept here rather than in app.js
// because app.js drives the DOM and cannot be imported.

test("a failing refresh is not current, whatever the stream is doing", () => {
  // The case that used to be invisible: the stream is up, the refresh is not.
  assert.deepEqual(boardState(true, true), { cls: "stale", text: "not current" });
  assert.deepEqual(boardState(false, true), { cls: "stale", text: "not current" });
  // And the two healthy readings are unchanged.
  assert.deepEqual(boardState(true, false), { cls: "live", text: "live" });
  assert.deepEqual(boardState(false, false), { cls: "down", text: "reconnecting…" });
});

test("the empty table says which of the three silences it is", () => {
  // Rows on screen: no message at all.
  assert.equal(emptyMessage(3, false, false, true), null);
  assert.equal(emptyMessage(3, true, true, true), null);
  // The operator's own filter.
  assert.equal(emptyMessage(0, true, false, true), "No sessions match the filter.");
  // A fleet with nothing in it — an answer.
  assert.equal(emptyMessage(0, false, false, true), "No sessions in view.");
  // Never reached the server: not an empty fleet, and it must not read as one.
  assert.equal(emptyMessage(0, false, true, false), "Cannot reach the server.");
  assert.equal(emptyMessage(0, true, true, false), "Cannot reach the server.");
  // Reached it before, cannot now: the emptiness is real but the list is not current.
  assert.equal(emptyMessage(0, false, true, true), "No sessions in view — and the list is not current.");
});

// #667. The dashboard grew the two things that let it call the operator: which
// sessions have just started calling, and which one to go to first.

test("the jump order matches the shared fixture, case for case", async () => {
  const f = await fixture("attention-order-cases.json");
  assert.ok(f.cases.length > 0, "the shared fixture carries no cases");
  for (const c of f.cases) {
    assert.equal(nextAttention(c.sessions), c.want, c.name);
  }
});

test("opening the dashboard is silent, and each entry notifies once", () => {
  const s = (id, attention, call_at) => ({ id, attention, call_at });
  const blocked = [s("a", true), s("b", false)];

  // First poll: nothing fires, however much of the fleet is already blocked.
  assert.deepEqual(enteredAttention(blocked, new Set(), false), []);

  // Armed now. `b` joins the set, `a` was already in it.
  let seen = attentionIds(blocked);
  assert.deepEqual(enteredAttention([s("a", true), s("b", true)], seen, true).map((x) => x.id), ["b"]);

  // Held state does not re-notify.
  seen = attentionIds([s("a", true), s("b", true)]);
  assert.deepEqual(enteredAttention([s("a", true), s("b", true)], seen, true), []);

  // Leaving the set re-arms it.
  seen = attentionIds([s("a", false), s("b", true)]);
  assert.deepEqual(enteredAttention([s("a", true), s("b", true)], seen, true).map((x) => x.id), ["a"]);

  // A raised call counts as calling even with no attention status (ADR-0010).
  assert.deepEqual(enteredAttention([s("c", false, "2026-09-01T10:00:00Z")], new Set(), true).map((x) => x.id), ["c"]);
});
