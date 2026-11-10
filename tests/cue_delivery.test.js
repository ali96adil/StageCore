"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const test = require("node:test");

const app = fs.readFileSync("internal/operatorweb/static/app.js", "utf8");
const dispatch = app.slice(app.indexOf("async function dispatchRuntimeCue("), app.indexOf("async function stopCueRuntime("));

function harness(api) {
  let id = 0;
  let rendered = 0;
  const ctx = {
    state: { page: "runtime", user: {}, project: { project_id: "project" } },
    api, requestID: () => String(++id),
    renderRuntime: async () => { rendered++; },
    errorMessage: (error) => error.message,
  };
  vm.runInNewContext(dispatch, ctx);
  return {
    state: ctx.state,
    send: () => ctx.dispatchRuntimeCue("project", { session: { session_id: "exact-session" }, current_cue: { cue_id: "exact-cue" } }, "go"),
    rendered: () => rendered,
  };
}

test("later request owns the displayed receipt even when the earlier GO completes last", async () => {
  const replies = [];
  const h = harness((path, options) => new Promise((resolve) => {
    assert.equal(path, "/api/v1/projects/project/runtime/go");
    assert.equal(options.json.expected_session_id, "exact-session");
    assert.equal(options.json.expected_current_cue_id, "exact-cue");
    replies.push(resolve);
  }));
  const earlier = h.send();
  const later = h.send();
  replies[1]({ result: { status: "COMPLETED", payload: { cue_execution_id: "later-execution" } } });
  await later;
  replies[0]({ result: { status: "COMPLETED", payload: { cue_execution_id: "earlier-execution" } } });
  await earlier;
  assert.equal(h.state.runtimeCueReceipt.requestID, "2");
  assert.equal(h.state.runtimeCueReceipt.result.payload.cue_execution_id, "later-execution");
  assert.equal(h.rendered(), 1);
});

test("a transport failure is unconfirmed and never automatically retries GO", async () => {
  let attempts = 0;
  const h = harness(async () => { attempts++; throw new TypeError("lost reply"); });
  await h.send();
  assert.equal(attempts, 1);
  assert.equal(h.state.runtimeCueReceipt.status, "UNCONFIRMED");
  assert.match(h.state.runtimeCueReceipt.message, /Session History/);
});
