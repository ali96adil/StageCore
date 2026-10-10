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
  extract("const runtimeUncertainCommandStorageKey =", "const state = {"),
  extract("async function renderRuntime(", "async function startRuntime("),
  extract("function loadRuntimeUncertainCommand(", "async function goRuntime("),
  extract("async function goRuntime(", "async function stopCueRuntime("),
  extract("async function jumpRuntime(", "async function stopSessionRuntime("),
  extract("function startRuntimePolling(", "window.addEventListener("),
].join("\n");

function fixture(storage = new Map()) {
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
    runtimeForceExitAvailable: false, runtimeTimer: null, runtimeUncertainCommand: null,
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
  const sessionStorage = {
    getItem: (key) => storage.has(key) ? storage.get(key) : null,
    setItem: (key, value) => storage.set(key, String(value)),
    removeItem: (key) => storage.delete(key),
  };
  const context = vm.createContext({
    state, content, sessionStorage, el: (id) => nodes.get(id) || null, globalMessage: freshNode(),
    api: async (path) => path.includes("/preflight") ? { status: "PASS", checks: [] } : runtime(),
    esc: (s) => String(s ?? ""), fmtDate: (v) => String(v ?? ""), pill: (s) => String(s),
    cueParentMapFor: () => new Map(), cueInList: (all, id) => all.find((c) => c.cue_id === id),
    cueRelationshipLabel: () => "", renderCueRelationship: () => "",
    canRuntime: () => true, groupRuntimeReadinessIssues: () => [],
    runtimeReadinessGroupTitle: () => "",
    setMessage: noop, errorMessage: (error) => error?.message || "Request failed",
    requestID: () => "id", confirm: () => true, prompt: () => "FORCE",
    navigate: noop, startRuntime: noop, stopCueRuntime: noop, setProjectBlackoutRuntime: noop,
    setEmergencyBlackoutRuntime: noop, stopSessionRuntime: noop, forceStopSessionRuntime: noop,
    document: { hidden: false }, setInterval: (callback) => { interval = callback; return 1; },
    clearInterval: noop,
  });
  vm.runInContext(functions, context, { filename: "app.js runtime functions" });
  return { context, state, nodes, runtime, content, storage, getHTML: () => html, getInterval: () => interval };
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
  // VM promises cross realms: flush the full microtask queue before asserting.
  await new Promise(setImmediate);
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

test("Ambiguous GO POST blocks the next GO until fresh cue verification and explicit acknowledgement", async () => {
  const f = fixture();
  let attempts = 0;
  let reads = 0;
  const confirmations = [];
  f.context.confirm = (message) => { confirmations.push(message); return false; };
  f.context.api = async (path, options) => {
    if (path.endsWith("/runtime/go") && options?.method === "POST") {
      attempts++;
      if (attempts === 1) throw new Error("network connection lost");
      return { result: { status: "ACCEPTED" } };
    }
    if (path.endsWith("/runtime")) reads++;
    return path.includes("/preflight") ? { status: "PASS", checks: [] } : f.runtime();
  };
  await f.context.goRuntime();
  assert.equal(attempts, 1);
  assert.equal(f.state.runtimeUncertainCommand.action, "GO");

  await f.context.renderRuntime();
  assert.match(f.getHTML(), /Previous GO response unknown/);
  await f.context.goRuntime();
  assert.equal(attempts, 1, "no second GO without acknowledgement");
  assert.ok(reads >= 2, "fresh runtime read before approval");
  assert.match(confirmations[0], /Check the physical stage/);
  assert.equal(f.state.runtimeUncertainCommand.action, "GO");

  f.context.confirm = () => true;
  await f.context.goRuntime();
  assert.equal(attempts, 2, "explicit acknowledgement permits one new GO");
  assert.equal(f.state.runtimeUncertainCommand, null);
});

test("Ambiguous Jump also prevents GO without a fresh explicit confirmation", async () => {
  const f = fixture();
  await f.context.renderRuntime();
  f.nodes.get("jumpCueSelect").value = "B";
  let jumps = 0;
  let gos = 0;
  let allow = true;
  f.context.confirm = () => allow;
  f.context.api = async (path, options) => {
    if (options?.method === "POST" && path.endsWith("/runtime/jump")) {
      jumps++;
      throw new Error("socket closed");
    }
    if (options?.method === "POST" && path.endsWith("/runtime/go")) gos++;
    return path.includes("/preflight") ? { status: "PASS", checks: [] }
      : path.endsWith("/runtime/go") ? { result: { status: "ACCEPTED" } } : f.runtime();
  };
  await f.context.jumpRuntime();
  assert.equal(jumps, 1);
  assert.equal(f.state.runtimeUncertainCommand.action, "JUMP");
  allow = false;
  await f.context.goRuntime();
  assert.equal(gos, 0, "cross-action blind retry must not advance next Cue");
  assert.equal(f.state.runtimeUncertainCommand.action, "JUMP");
});

test("Ambiguity banner is scoped to Runtime instead of the Projects template", () => {
  const projects = source.indexOf("function renderProjects()");
  const runtime = source.indexOf("async function renderRuntime(");
  assert.ok(projects >= 0 && runtime > projects);
  assert.doesNotMatch(source.slice(projects, runtime), /runtime-unverified-command/);
  assert.match(source.slice(runtime), /runtime-unverified-command/);
});

test("A tab reload while GO POST is unresolved preserves confirmation guard", async () => {
  const f = fixture();
  let release;
  let sends = 0;
  f.context.api = (path, options) => {
    if (options?.method === "POST" && path.endsWith("/runtime/go")) {
      sends++;
      return new Promise((resolve) => { release = resolve; });
    }
    return Promise.resolve(path.includes("/preflight") ? { status: "PASS", checks: [] } : f.runtime());
  };
  const original = f.context.goRuntime();
  await new Promise(setImmediate);
  assert.equal(sends, 1);
  const saved = [...f.storage.values()].find((value) => value.includes('"action":"GO"'));
  assert.ok(saved, "pending GO must be written before network response");

  const reloaded = fixture(f.storage);
  reloaded.state.runtimeUncertainCommand = reloaded.context.loadRuntimeUncertainCommand();
  assert.equal(reloaded.state.runtimeUncertainCommand.action, "GO");
  await reloaded.context.renderRuntime();
  assert.match(reloaded.getHTML(), /Previous GO response unknown/);

  let secondSends = 0;
  reloaded.context.confirm = () => false;
  reloaded.context.api = async (path, options) => {
    if (options?.method === "POST" && path.endsWith("/runtime/go")) secondSends++;
    return path.includes("/preflight") ? { status: "PASS", checks: [] } : reloaded.runtime();
  };
  await reloaded.context.goRuntime();
  assert.equal(secondSends, 0, "reloaded tab must not blindly advance Cue");
  release({ result: { status: "ACCEPTED" } });
  await original;
});

test("Completed response clears the persisted guard and malformed storage is ignored", async () => {
  const storage = new Map();
  const f = fixture(storage);
  f.context.api = async (path, options) => {
    if (options?.method === "POST" && path.endsWith("/runtime/go"))
      return { result: { status: "ACCEPTED" } };
    return path.includes("/preflight") ? { status: "PASS", checks: [] } : f.runtime();
  };
  await f.context.goRuntime();
  assert.equal(f.state.runtimeUncertainCommand, null);
  assert.equal(f.context.loadRuntimeUncertainCommand(), null);
  assert.equal(storage.size, 0);
  storage.set("stagecore.runtime.uncertain_command.v1", "{invalid");
  assert.equal(f.context.loadRuntimeUncertainCommand(), null);
});

test("Persistence bootstrap key must exist before state restoration", () => {
  const key = source.indexOf("const runtimeUncertainCommandStorageKey =");
  const stateInit = source.indexOf("const state = {");
  assert.ok(key >= 0 && stateInit > key, "restoring state must not hit temporal dead zone");
});

test("Switching Projects cannot overwrite an unresolved GO from another Project", async () => {
  const f = fixture();
  f.state.runtimeUncertainCommand = { projectID: "other-project", action: "GO" };
  let sent = 0;
  f.context.api = async (path, options) => {
    if (options?.method === "POST") sent++;
    return path.includes("/preflight") ? { status: "PASS", checks: [] } : f.runtime();
  };
  await f.context.renderRuntime();
  assert.match(f.getHTML(), /Previous GO response unknown/);
  assert.match(f.getHTML(), /other-project/);
  await f.context.goRuntime();
  assert.equal(sent, 0, "different Project cannot erase or bypass pending GO");
  assert.equal(f.state.runtimeUncertainCommand.projectID, "other-project");
});

test("GO does not dispatch after navigating away while Runtime GET is unresolved", async () => {
  const f = fixture();
  let resolveRead;
  let sent = 0;
  f.context.api = (path, options) => {
    if (options?.method === "POST") { sent++; return Promise.resolve({ result: { status: "ACCEPTED" } }); }
    if (path.endsWith("/runtime")) return new Promise((resolve) => { resolveRead = resolve; });
    return Promise.resolve({ status: "PASS", checks: [] });
  };
  const action = f.context.goRuntime();
  f.state.page = "projects";
  resolveRead(f.runtime());
  await action;
  assert.equal(sent, 0, "late runtime GET must never dispatch after leaving page");
  assert.equal(f.state.runtimeUncertainCommand, null);
});

test("Jump cannot submit to an earlier Project after Project change during GET", async () => {
  const f = fixture();
  await f.context.renderRuntime();
  f.nodes.get("jumpCueSelect").value = "B";
  let resolveRead;
  let sent = 0;
  f.context.api = (path, options) => {
    if (options?.method === "POST") { sent++; return Promise.resolve({ result: { status: "ACCEPTED" } }); }
    if (path.endsWith("/runtime")) return new Promise((resolve) => { resolveRead = resolve; });
    return Promise.resolve({ status: "PASS", checks: [] });
  };
  const action = f.context.jumpRuntime();
  f.state.project = { project_id: "other-project" };
  resolveRead(f.runtime());
  await action;
  assert.equal(sent, 0, "late Jump GET must never send to old Project");
  assert.equal(f.state.runtimeUncertainCommand, null);
});
