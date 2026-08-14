// Tests for the dashboard's pure helpers (internal/web/static/lib.js).
//
// They import the shipped file directly — no copy, no build step — so a test can
// only pass against the code the daemon actually embeds.

import assert from "node:assert/strict";
import { test } from "node:test";
import { readFile } from "node:fs/promises";

import {
  esc, dash, trim, hasCall, detailText, humanTokens, relAge, relResetHint,
  shortModel, projectName, totalTokens, sparkSVG, migrateKeys, fullColOrder, colHidden,
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
