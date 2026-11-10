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
        { session_id: session.session_id, observation_count: 4, selection_mode: "AUTO", eligible },
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
