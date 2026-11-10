"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const test = require("node:test");

const memory = fs.readFileSync("internal/operatorweb/static/memory.js", "utf8");
const approval = memory.slice(memory.indexOf("async function renderRehearsalApproval("), memory.indexOf("async function renderSessionDetail("));
const session = { session_id: "rehearsal A", runtime_snapshot_id: "historic snapshot" };

function harness(eligible) {
  const calls = [];
  let change;
  const select = { addEventListener: (_, handler) => { change = handler; } };
  const container = { isConnected: true, querySelector: () => eligible ? select : null };
  const ctx = {
    state: { user: {}, project: { project_id: "project A" }, page: "sessions" },
    esc: String, f028CanEditSelection: () => true,
    api: async (url, options) => {
      calls.push([url, options]);
      return { report: { sessions: [
        { session_id: "other", selection_mode: "EXCLUDE", eligible: true },
        { session_id: session.session_id, runtime_snapshot_id: session.runtime_snapshot_id, observation_count: 4, selection_mode: "AUTO", eligible },
      ] } };
    },
  };
  vm.runInNewContext(approval, ctx);
  return { calls, ctx, container, select, open: () => ctx.renderRehearsalApproval(container, "project A", session), change: () => change() };
}

test("approval reads the historical snapshot and writes only the reviewed rehearsal", async () => {
  const h = harness(true);
  await h.open();
  assert.match(h.calls[0][0], /runtime_snapshot_id=historic%20snapshot&session_id=rehearsal%20A$/);
  assert.equal(h.select.value, "AUTO");
  h.select.value = "INCLUDE";
  await h.change();
  assert.match(h.calls[1][0], /projects\/project%20A\/timing-intelligence\/sessions\/rehearsal%20A$/);
  assert.equal(h.calls[1][1].json.mode, "INCLUDE");
  assert.equal(h.calls[1][1].json.review.mode, "AUTO");
  assert.equal(h.calls[1][1].json.review.runtime_snapshot_id, session.runtime_snapshot_id);
});

test("a detached review cannot submit and an ineligible rehearsal is read-only", async () => {
  const h = harness(true);
  await h.open();
  h.container.isConnected = false;
  await h.change();
  assert.equal(h.calls.length, 1);
  const readOnly = harness(false);
  await readOnly.open();
  assert.doesNotMatch(readOnly.container.innerHTML, /<select>/);
});

test("an older approval report cannot overwrite the latest rehearsal review", async () => {
  const pending = [];
  const container = { isConnected: true, innerHTML: "", querySelector: () => null };
  const ctx = {
    state: { user: {}, project: { project_id: "project A" }, page: "sessions" },
    esc: String,
    f028CanEditSelection: () => false,
    api: () => new Promise(resolve => pending.push(resolve)),
  };
  vm.runInNewContext(approval, ctx);
  const first = ctx.renderRehearsalApproval(container, "project A", session);
  const second = ctx.renderRehearsalApproval(container, "project A", session);
  assert.equal(pending.length, 2);
  const response = (mode) => ({ report: { sessions: [{
    session_id: session.session_id, runtime_snapshot_id: session.runtime_snapshot_id,
    observation_count: 4, selection_mode: mode, eligible: true, effective: mode === "INCLUDE"
  }] } });
  pending[1](response("INCLUDE"));
  await second;
  assert.match(container.innerHTML, /Selection: INCLUDE/);
  const latestHTML = container.innerHTML;
  pending[0](response("AUTO"));
  await first;
  assert.equal(container.innerHTML, latestHTML, "stale approval replaced latest review");
});

test("session-history approval ignores a late report after switching projects", async () => {
  const h = harness(true);
  h.container.innerHTML = "";
  let deliver;
  h.ctx.api = () => new Promise(resolve => { deliver = resolve; });
  const pending = h.open();
  assert.equal(typeof deliver, "function");
  h.ctx.state.project = { project_id: "project B" };
  deliver({ report: { sessions: [{
    session_id: session.session_id, runtime_snapshot_id: session.runtime_snapshot_id,
    observation_count: 3, selection_mode: "INCLUDE", eligible: true,
  }] } });
  await pending;
  assert.equal(h.container.innerHTML, "", "old Project timing evidence appeared after switch");
});

test("session-history approval ignores a late report after switching users", async () => {
  const h = harness(true);
  h.container.innerHTML = "";
  let deliver;
  h.ctx.api = () => new Promise(resolve => { deliver = resolve; });
  const pending = h.open();
  h.ctx.state.user = { user_id: "another-user" };
  deliver({ report: { sessions: [{
    session_id: session.session_id, runtime_snapshot_id: session.runtime_snapshot_id,
    observation_count: 3, selection_mode: "INCLUDE", eligible: true,
  }] } });
  await pending;
  assert.equal(h.container.innerHTML, "", "old user's approval evidence appeared after switch");
});

test("switching user or Project disables stale approval controls before mutation", async () => {
  for (const switchScope of [
    h => { h.ctx.state.project = { project_id: "project B" }; },
    h => { h.ctx.state.user = { user_id: "another-user" }; },
  ]) {
    const h = harness(true);
    await h.open();
    h.select.value = "INCLUDE";
    switchScope(h);
    await h.change();
    assert.equal(h.calls.length, 1, "old review sent a write after scope switch");
  }
});
