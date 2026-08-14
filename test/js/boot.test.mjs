// A smoke test: does the dashboard still start?
//
// app.js drives the DOM, so it cannot be unit-tested — but it can be *loaded*.
// This stubs just enough of the browser for its top-level code to run, then
// imports the shipped file. It deliberately asserts almost nothing: the only
// claim is "importing app.js does not throw".
//
// What that is worth catching: a syntax error, an import that does not resolve, a
// name moved to lib.js but never exported, a top-level call to something that no
// longer exists. Any of those leaves the daemon serving a 200 for a page that is
// blank in the browser, and nothing else in CI would notice (#430).
//
// What it is NOT: evidence that anything renders correctly. The stub answers
// every property access with a permissive object, so behaviour asserted against
// it would be asserted against a fiction. Behaviour lives in lib.js and is tested
// in dashboard.test.mjs against the real functions.

import assert from "node:assert/strict";
import { test } from "node:test";

function stubBrowser() {
  const node = new Proxy(function () {}, {
    get: (_t, k) =>
      k === "classList" || k === "style" || k === "dataset" ? node
        : k === "hidden" || k === "value" || k === "innerHTML" || k === "textContent" ? ""
          : k === "children" || k === "options" ? []
            : k === Symbol.toPrimitive || k === "toString" ? () => ""
              : node,
    set: () => true,
    apply: () => node,
    has: () => true,
  });

  globalThis.document = {
    getElementById: () => node,
    querySelectorAll: () => [],
    createElement: () => node,
    addEventListener: () => {},
    documentElement: node,
    body: node,
  };
  globalThis.localStorage = { getItem: () => null, setItem() {}, removeItem() {} };
  globalThis.window = globalThis;
  globalThis.fetch = async () => ({ ok: true, status: 200, json: async () => [], headers: { get: () => null } });
  globalThis.EventSource = class { close() {} addEventListener() {} };
  globalThis.matchMedia = () => ({ matches: false, addEventListener() {} });
}

test("the dashboard module loads and its top-level code runs", async () => {
  stubBrowser();
  const url = new URL("../../internal/web/static/app.js", import.meta.url);
  await assert.doesNotReject(() => import(url), "importing app.js threw — the page would be blank");
});
