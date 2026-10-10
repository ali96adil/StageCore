"use strict";
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const { test } = require("node:test");

const source = fs.readFileSync("internal/operatorweb/static/app.js", "utf8");
const start = source.indexOf("async function renderRuntime(startPolling = false) {");
const end = source.indexOf("async function startRuntime(mode) {", start);
assert.ok(start >= 0 && end > start, "renderRuntime function available");

function report() { return { status: "PASS", checks: [] }; }
function runtime(label, active = true) {
  return {
    project: { name: "StageCore" }, mode: active ? "REHEARSAL" : "EDIT",
    session: active ? { session_id: "session-1", type: "REHEARSAL", status: "ACTIVE" } : null,
    current_cue: active ? { cue_id: label, display_label: "Q", name: label } : null,
    next_cue: active ? { cue_id: "cue-2", display_label: "Q2", name: "NEXT" } : null,
    cues: [
      { cue_id: label, display_label: "Q", name: label },
      { cue_id: "cue-2", display_label: "Q2", name: "NEXT" },
    ],
    runtime_snapshot: { snapshot_version: 5, runtime_snapshot_id: "published-1" },
    managed_output_blackout: false, running_executions: [], recent_output_failures: [],
  };
}
function harness(api) {
  const state = {
    project: { project_id: "project-a" }, page: "runtime", runtimeRenderGeneration: 0,
    runtimeJumpSelection: null, runtimeGoUncertain: null, runtimeGoFlight: false,
    runtimeLastRefreshAt: 0, runtimeLastRefreshError: "",
  };
  let lastSelect = null;
  let html = "";
  const content = {
    get innerHTML() { return html; },
    set innerHTML(value) {
      html = value;
      lastSelect = {
        value: "", addEventListener: (type, cb) => {
          if (type === "change") lastSelect.onchange = cb;
        },
      };
    },
  };
  const sandbox = {
    state, api, content,
    el: (id) => id === "jumpCueSelect" ? lastSelect : null,
    canRuntime: () => true,
    cueParentMapFor: () => new Map(),
    cueInList: (cues, id) => cues.find(c => c.cue_id === id),
    cueRelationshipLabel: () => "",
    renderCueRelationship: () => "",
    groupRuntimeReadinessIssues: () => [],
    runtimeReadinessGroupTitle: () => "",
    pill: (text) => String(text),
    esc: (v) => String(v ?? ""),
    errorMessage: (error) => error.message,
    fmtDate: () => "-",
    startRuntimePolling: () => {},
  };
  const renderRuntime = vm.runInNewContext(source.slice(start, end) + "\nrenderRuntime;", sandbox);
  return { state, content, renderRuntime, get lastSelect() { return lastSelect; } };
}

test("a late older poll cannot replace newer Cue state", async () => {
  let release;
  const oldest = new Promise((resolve) => { release = resolve; });
  let calls = 0;
  const h = harness(async (path) => {
    if (path.endsWith("/preflight")) return report();
    return ++calls === 1 ? oldest : runtime("NEW_CUE");
  });
  const first = h.renderRuntime();
  await h.renderRuntime();
  assert.match(h.content.innerHTML, /NEW_CUE/);
  release(runtime("OLD_CUE"));
  await first;
  assert.match(h.content.innerHTML, /NEW_CUE/);
  assert.doesNotMatch(h.content.innerHTML, /OLD_CUE/);
});

test("a Preflight API failure is UNKNOWN and cannot enable SHOW", async () => {
  const h = harness(async (path) => {
    if (path.endsWith("/preflight")) throw new Error("preflight connection failed");
    return runtime("NO_SESSION", false);
  });
  await h.renderRuntime();
  assert.match(h.content.innerHTML, /Preflight: UNAVAILABLE/);
  assert.match(h.content.innerHTML, /Preflight is unavailable/);
  assert.match(h.content.innerHTML, /id="startShowButton"[^>]*disabled/);
  assert.doesNotMatch(h.content.innerHTML, /No current Preflight issues/);
});

test("Jump selection survives Runtime poll redraw in same Session", async () => {
  const h = harness(async (path) => path.endsWith("/preflight") ? report() : runtime("CUE_ONE"));
  await h.renderRuntime();
  const select = h.lastSelect;
  select.value = "cue-2";
  select.onchange();
  await h.renderRuntime();
  assert.equal(h.lastSelect.value, "cue-2");
  assert.equal(h.state.runtimeJumpSelection.cueID, "cue-2");
});

test("a late poll after navigation never redraws another page", async () => {
  let release;
  const held = new Promise((resolve) => { release = resolve; });
  const h = harness(async (path) => path.endsWith("/preflight") ? report() : held);
  const pending = h.renderRuntime();
  h.state.page = "projects";
  release(runtime("SHOULD_NOT_RENDER"));
  await pending;
  assert.equal(h.content.innerHTML, "");
});
