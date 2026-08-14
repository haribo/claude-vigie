// Tests for the GNOME indicator's pure helpers (gnome-extension/lib.js).
//
// They import the shipped file directly, so what passes here is what GNOME Shell
// loads. extension.js itself cannot be imported outside gjs — it pulls in
// `gi://` and `resource:///org/gnome/shell/...` — which is precisely why the
// logic worth checking lives in lib.js.

import assert from "node:assert/strict";
import { test } from "node:test";

import { groupOrder, basename } from "../../gnome-extension/lib.js";

// The documented vocabulary, passed in explicitly. The list that ships is checked
// against docs/design/session-status.md by a Go test (#423); what is checked here
// is the behaviour around it.
const ORDER = ["working", "thinking", "compacting", "waiting", "stalled", "idle", "error", "stale", "ended"];
const S = (...statuses) => statuses.map((status) => ({ status }));

test("known statuses come back in the documented order", () => {
  assert.deepEqual(groupOrder(S("idle", "working"), ORDER), ORDER);
  assert.deepEqual(groupOrder([], ORDER), ORDER);
});

// This is the #422 guarantee. The menu used to iterate a fixed list, so a session
// whose status was not on it matched no group and vanished from the menu with
// nothing to say so. A status added server-side must appear, styled or not.
test("a status the extension does not know is appended, never dropped", () => {
  assert.deepEqual(groupOrder(S("working", "quantum"), ORDER), [...ORDER, "quantum"]);
});

test("several unknown statuses are ordered stably and de-duplicated", () => {
  assert.deepEqual(groupOrder(S("zeta", "alpha", "zeta"), ORDER), [...ORDER, "alpha", "zeta"]);
});

test("a missing or empty status creates no phantom group", () => {
  assert.deepEqual(groupOrder([{ status: "" }, {}], ORDER), ORDER);
});

// A shorter known list is what the extension actually shipped before #422: the
// point is that the sessions still surface.
test("even a truncated known list loses no session", () => {
  const short = ["working", "idle"];
  const got = groupOrder(S("working", "stalled", "idle"), short);
  assert.ok(got.includes("stalled"), `stalled was dropped: ${JSON.stringify(got)}`);
});

test("basename reduces a project path to its last segment", () => {
  assert.equal(basename("/home/u/dev/api-gateway"), "api-gateway");
  assert.equal(basename("/home/u/dev/api-gateway/"), "api-gateway");
  assert.equal(basename(""), "");
  assert.equal(basename(null), "");
});
