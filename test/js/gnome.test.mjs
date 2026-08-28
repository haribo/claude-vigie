// Tests for the GNOME indicator's pure helpers (gnome-extension/lib.js).
//
// They import the shipped file directly, so what passes here is what GNOME Shell
// loads. extension.js itself cannot be imported outside gjs — it pulls in
// `gi://` and `resource:///org/gnome/shell/...` — which is precisely why the
// logic worth checking lives in lib.js.

import assert from "node:assert/strict";
import { test } from "node:test";
import { readFile } from "node:fs/promises";

import { groupOrder, needsAttention, attentionReason, attentionIds, STATUS_ORDER } from "../../gnome-extension/lib.js";

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

// #466. The indicator exists so an operator who is not looking at the TUI still
// learns that a session needs them — and it reacted to one signal out of four:
// `waiting`. A stalled turn raised nothing, and a session's own call, the headline
// of 0.5.0, did not exist for it at all.

test("a call counts, whatever the status underneath", () => {
  assert.equal(needsAttention({ status: "idle", call_at: "t" }), true);
  assert.equal(needsAttention({ status: "working", call_at: "t" }), true,
    "a session can raise a call mid-turn");
  assert.equal(needsAttention({ status: "idle" }), false);
});

test("the daemon's verdict is honoured, whatever the status beside it", () => {
  // This indicator used to carry its own copy of the blocking statuses, and #466
  // exists because a copy that disagrees with the TUI about when to interrupt you
  // is worse than no indicator. The daemon sends the verdict now (ADR-0011,
  // #617), so the only thing left to get wrong here is ignoring it.
  for (const status of ["waiting", "stalled", "error"]) {
    assert.equal(needsAttention({ status, attention: true }), true, `${status} must call the operator`);
  }
  for (const status of ["working", "thinking", "compacting", "idle", "stale", "ended"]) {
    assert.equal(needsAttention({ status, attention: false }), false, `${status} must not interrupt`);
  }
});

test("needsAttention tolerates a missing session", () => {
  assert.equal(needsAttention(null), false);
  assert.equal(needsAttention(undefined), false);
  assert.equal(needsAttention({}), false);
});

// The body has to say why: a stalled turn, an API error and a raised call all want
// different things, and one wording for all three would misinform.
test("the reason distinguishes the four signals", () => {
  assert.equal(attentionReason({ status: "idle", call_at: "t", call_message: "build done" }), "build done");
  assert.equal(attentionReason({ status: "idle", call_at: "t" }), "called you",
    "a call with no message is still a call");
  assert.match(attentionReason({ status: "waiting" }), /waiting/);
  assert.match(attentionReason({ status: "stalled" }), /stalled/);
  assert.match(attentionReason({ status: "error" }), /error/);
  assert.equal(attentionReason({ status: "working" }), "");
});

test("a call outranks the status it rides on", () => {
  assert.equal(attentionReason({ status: "waiting", call_at: "t", call_message: "done" }), "done",
    "the session speaking beats an inference about it");
});

// The notification is edge-triggered off this set: anything already in it must not
// re-notify on the next poll.
test("attentionIds holds exactly the sessions calling for the operator", () => {
  const sessions = [
    { id: "a", status: "waiting", attention: true },
    { id: "b", status: "working", attention: false },
    { id: "c", status: "stalled", attention: true },
    { id: "d", status: "idle", attention: false, call_at: "t" },
    { id: "e", status: "ended", attention: false },
  ];
  assert.deepEqual([...attentionIds(sessions)].sort(), ["a", "c", "d"]);
  assert.equal(attentionIds([]).size, 0);
  assert.equal(attentionIds(null).size, 0);
});

test("the indicator's status order agrees with the shared fixture", async () => {
  const raw = await readFile(new URL("../fixtures/status-vocabulary.json", import.meta.url), "utf8");
  assert.deepEqual(STATUS_ORDER, JSON.parse(raw).order,
    "a status missing here takes its sessions off the menu entirely (#422)");
});
