"use strict";
// Behavior-level runtime UI tests: no network, no browser dependency.
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const source = fs.readFileSync(path.join(__dirname, "../internal/operatorweb/static/app.js"), "utf8");
function extract(begin, end) {
  const i = source.indexOf(begin);
  const j = source.indexOf(end, i + begin.length);
  assert.ok(i >= 0 && j > i, "Cannot isolate runtime function " + begin);
  return source.slice(i, j);
}
const functions = [
  extract("async function renderRuntime(", "async function startRuntime("),
  extract("async function goRuntime(", "async function stopCueRuntime("),
  extract("async function jumpRuntime(", "async function stopSessionRuntime("),
  extract("function startRuntimePolling(", "window.addEventListener("),
].join("\n");

function fixture() {
  const nodes = new Map();
  const freshNode = (value = "") => ({
    value, disabled: false, options: [], textContent: "", className: "",
    addEventListener() {},
  });
  let html = "";
  const content = {
    set innerHTML(next) {
      html = String(next);
      for (const id of ["goButton", "jumpButton", "startShowButton", "runtimeFreshness"]) {
        if (html.includes('id="' + id + '"')) nodes.set(id, freshNode());
      }
      const options = [{ value: "" }];
      const block = html.match(/<select id="jumpCueSelect">([\s\S]*?)<\/select>/);
      if (block) {
        for (const m of block[1].matchAll(/<option value="([^"]*)"/g)) options.push({ value: m[1] });
        const select = freshNode();
        select.options = options;
        nodes.set("jumpCueSelect", select);
      } else {
        nodes.delete("jumpCueSelect");
      }
    },
    get innerHTML() { return html; },
  };
  const state = {
    project: { project_id: "p" }, page: "runtime", user: { role: "OWNER" },
    runtimeRenderGeneration: 0, runtimeRefreshPending: false, runtimeActionInFlight: false,
    runtimeForceExitAvailable: false, runtimeTimer: null,
  };
  const cue = (id) => ({ cue_id: id, display_label: id, name: "Cue " + id, enabled: true });
  const runtime = () => ({
    project: { name: "Test Show" }, runtime_snapshot: { snapshot_version: 1, runtime_snapshot_id: "sn" },
    session: { session_id: "s", type: "REHEARSAL", status: "ACTIVE", started_at: "2026-10-10T12:00:00Z" },
    mode: "REHEARSAL", current_cue: cue("A"), next_cue: cue("B"), cues: [cue("A"), cue("B")],
    running_executions: [], managed_output_blackout: false, recent_action_failures: [],
  });
  let interval;
  const noop = () => {};
  const context = vm.createContext({
    state, content, el: (id) => nodes.get(id) || null, globalMessage: freshNode(),
    api: async (path) => path.includes("/preflight") ? { status: "PASS", checks: [] } : runtime(),
    esc: (s) => String(s ?? ""), fmtDate: (v) => String(v ?? ""), pill: (s) => String(s),
    cueParentMapFor: () => new Map(), cueInList: (all, id) => all.find((c) => c.cue_id === id),
    cueRelationshipLabel: () => "", renderCueRelationship: () => "",
    canRuntime: () => true, groupRuntimeReadinessIssues: () => [],
    runtimeReadinessGroupTitle: () => "",
    setMessage: noop, requestID: () => "id", confirm: () => true, prompt: () => "FORCE",
    navigate: noop, startRuntime: noop, stopCueRuntime: noop, setProjectBlackoutRuntime: noop,
    setEmergencyBlackoutRuntime: noop, stopSessionRuntime: noop, forceStopSessionRuntime: noop,
    document: { hidden: false }, setInterval: (callback) => { interval = callback; return 1; },
    clearInterval: noop,
  });
  vm.runInContext(functions, context, { filename: "app.js runtime functions" });
  return { context, state, nodes, runtime, content, getHTML: () => html, getInterval: () => interval };
}

test("Preserve Jump target during polling and persist failed output in the rendered session", async () => {
  const f = fixture();
  await f.context.renderRuntime();
  f.nodes.get("jumpCueSelect").value = "B";
  f.context.api = async (p) => p.includes("/preflight") ? { status: "PASS", checks: [] } : {
    ...f.runtime(),
    recent_action_failures: [{
      cue_id: "A", action_id: "failed-1", result: "FAILED",
      error_code: "NO_DEVICE", response_summary: "output offline", started_at: "2026-10-10T12:00:00Z",
    }],
  };
  await f.context.renderRuntime();
  assert.equal(f.nodes.get("jumpCueSelect").value, "B");
  assert.match(f.getHTML(), /Recent output failures/);
  assert.match(f.getHTML(), /NO_DEVICE/);
  assert.match(f.getHTML(), /output offline/);
});

test("Preflight read errors are UNKNOWN, never READY; new SHOW entry blocked", async () => {
  const f = fixture();
  f.context.api = async (p) => {
    if (p.includes("/preflight")) throw new Error("offline");
    return { ...f.runtime(), session: null, mode: "EDIT" };
  };
  await f.context.renderRuntime();
  assert.match(f.getHTML(), /Preflight unavailable — readiness UNKNOWN/);
  assert.match(f.getHTML(), /id="startShowButton"[^>]*disabled/);
  assert.doesNotMatch(f.getHTML(), /No current Preflight issues/);
});

test("An older slow Runtime response cannot overwrite a newer one", async () => {
  const f = fixture();
  const resolves = [];
  f.context.api = (p) => p.includes("/preflight") ? Promise.resolve({ status: "PASS", checks: [] })
    : new Promise((resolve) => resolves.push(resolve));
  const older = f.context.renderRuntime();
  const newer = f.context.renderRuntime();
  resolves[1]({ ...f.runtime(), next_cue: { cue_id: "B", display_label: "B", name: "CURRENT NEW" } });
  await newer;
  resolves[0]({ ...f.runtime(), next_cue: { cue_id: "A", display_label: "A", name: "OLD STALE" } });
  await older;
  assert.match(f.getHTML(), /CURRENT NEW/);
  assert.doesNotMatch(f.getHTML(), /OLD STALE/);
});

test("Two GO calls during the same unresolved request submit only one command", async () => {
  const f = fixture();
  let posted = 0;
  let release;
  f.context.api = (p, options) => {
    if (options?.method === "POST" && p.endsWith("/runtime/go")) {
      posted++;
      return new Promise((resolve) => { release = resolve; });
    }
    return Promise.resolve(p.includes("/preflight") ? { status: "PASS", checks: [] } : f.runtime());
  };
  const first = f.context.goRuntime();
  const second = f.context.goRuntime();
  await Promise.resolve();
  await Promise.resolve();
  assert.equal(posted, 1);
  release({ result: { status: "ACCEPTED" } });
  await Promise.all([first, second]);
  assert.equal(posted, 1);
  assert.equal(f.state.runtimeActionInFlight, false);
});

test("Polling failure disables GO/Jump but does not disable STOP or blackout", async () => {
  const f = fixture();
  await f.context.renderRuntime();
  f.context.startRuntimePolling();
  f.context.api = async () => { throw new Error("Disconnected"); };
  await f.getInterval()();
  assert.equal(f.nodes.get("goButton").disabled, true);
  assert.equal(f.nodes.get("jumpButton").disabled, true);
  assert.match(f.nodes.get("runtimeFreshness").textContent, /Hub refresh FAILED/);
  assert.match(f.getHTML(), /STOP LATEST CUE/);
  assert.match(f.getHTML(), /EMERGENCY BLACKOUT/);
});
