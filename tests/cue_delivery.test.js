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
    send: (operation = "go", extra) => ctx.dispatchRuntimeCue("project", { session: { session_id: "exact-session" }, current_cue: { cue_id: "exact-cue" } }, operation, extra),
    rendered: () => rendered,
  };
}

test("later request owns the displayed receipt even when the earlier GO completes last", async () => {
  const replies = [];
  const h = harness((path, options) => new Promise((resolve) => {
    assert.equal(path, "/api/v1/projects/project/runtime/" + (replies.length ? "jump" : "go"));
    if (replies.length) assert.equal(options.json.cue_id, "jump-target");
    assert.equal(options.json.expected_session_id, "exact-session");
    assert.equal(options.json.expected_current_cue_id, "exact-cue");
    replies.push(resolve);
  }));
  const earlier = h.send();
  const later = h.send("jump", {cue_id: "jump-target", confirm: true});
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

test("a lost GO reply keeps execution history usable and late history replies cannot replace the selected trace", async () => {
  const executions = [];
  const h = harness(async () => {
    executions.push({cue_execution_id: "lost-go", name: "Committed GO"});
    throw new TypeError("connection lost after commit");
  });
  await h.send();
  assert.equal(h.state.runtimeCueReceipt.status, "UNCONFIRMED");

  const memory = fs.readFileSync("internal/operatorweb/static/memory.js", "utf8");
  const trace = memory.slice(memory.indexOf("function actionTraceCard("), memory.indexOf("async function loadNoteChoices("));
  const replies = [];
  const content = {};
  const ctx = { state: h.state, content, esc: String, fmtDate: String, pill: String,
    memoryKind: String, el: (id) => id === "backSessions" ? {addEventListener() {}} : null,
    api: (path) => new Promise((resolve) => {
      assert.equal(path, "/api/v1/projects/project/sessions/exact-session");
      replies.push(resolve);
    }),
  };
  h.state.page = "sessions";
  executions.push({cue_execution_id: "jump", name: "Selected Jump"});
  vm.runInNewContext(trace, ctx);
  const stale = ctx.renderSessionDetail("exact-session");
  const selected = ctx.renderSessionDetail("exact-session", "jump");
  const payload = {session: {session_id: "exact-session", session_type: "REHEARSAL"}, cue_executions: executions};
  replies[1](payload);
  await selected;
  replies[0](payload);
  await stale;
  assert.match(content.innerHTML, /Selected Jump/);
  assert.doesNotMatch(content.innerHTML, /Committed GO/);
  const history = ctx.renderSessionDetail("exact-session");
  replies[2](payload);
  await history;
  assert.match(content.innerHTML, /Committed GO/);
});
