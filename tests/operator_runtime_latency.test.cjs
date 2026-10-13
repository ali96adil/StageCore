"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const app = fs.readFileSync(path.join(__dirname, "../internal/operatorweb/static/app.js"), "utf8");
const preflight = fs.readFileSync(path.join(__dirname, "../internal/operatorweb/static/preflight.js"), "utf8");

function section(text, start, end) {
  const from = text.indexOf(start);
  const to = text.indexOf(end, from + start.length);
  assert.ok(from >= 0 && to > from, "Cannot extract " + start);
  return text.slice(from, to);
}

async function drain() {
  for (let i = 0; i < 8; i++) await Promise.resolve();
}

test("Preflight is single-flight, cached, and rechecked without blocking Runtime", async () => {
  const code = section(app, "function runtimePreflightSnapshot(projectID) {", "async function renderRuntime(startPolling = false) {");
  const state = {
    page: "runtime", project: { project_id: "p1" },
    runtimePreflightProjectID: null, runtimePreflightReport: null,
    runtimePreflightCheckedAt: 0, runtimePreflightPending: null, runtimePreflightEpoch: 0,
  };
  let clock = 1000;
  const requests = [];
  let repaints = 0;
  const api = (url) => new Promise((resolve, reject) => requests.push({ url, resolve, reject }));
  const run = new Function("state", "api", "renderRuntime", "Date", code + "\nreturn { snapshot: runtimePreflightSnapshot, record: recordRuntimePreflight };")(
    state, api, () => { repaints++; return Promise.resolve(); }, { now: () => clock }
  );

  assert.equal(run.snapshot("p1"), null);
  for (let i = 0; i < 10; i++) run.snapshot("p1");
  assert.equal(requests.length, 1, "polls must not multiply full readiness scans");
  requests[0].resolve({ status: "WARN", runtime_snapshot_id: "snap1" });
  await drain();
  assert.equal(run.snapshot("p1").status, "WARN");
  assert.equal(requests.length, 1);
  assert.equal(repaints, 1);

  clock += 10001;
  run.snapshot("p1");
  assert.equal(requests.length, 2, "a fresh scan starts after TTL");
  run.record("p1", { status: "PASS", runtime_snapshot_id: "snap1" });
  requests[1].resolve({ status: "BLOCK", runtime_snapshot_id: "snap1" });
  await drain();
  assert.equal(run.snapshot("p1").status, "PASS", "old background reply must not override a newer manual Preflight");

  state.project = { project_id: "p2" };
  assert.equal(run.snapshot("p2"), null, "switching Project must invalidate cached readiness");
  assert.equal(requests.length, 3);
  requests[2].resolve({ status: "BLOCK", runtime_snapshot_id: "snap2" });
  await drain();
  assert.equal(run.snapshot("p2").status, "BLOCK");
});

test("failed scan never reports READY and can be retried", async () => {
  const code = section(app, "function runtimePreflightSnapshot(projectID) {", "async function renderRuntime(startPolling = false) {");
  const state = {
    page: "runtime", project: { project_id: "p1" },
    runtimePreflightProjectID: null, runtimePreflightReport: null,
    runtimePreflightCheckedAt: 0, runtimePreflightPending: null, runtimePreflightEpoch: 0,
  };
  const requests = [];
  let clock = 1000;
  const api = (url) => new Promise((resolve, reject) => requests.push({ resolve, reject }));
  const run = new Function("state", "api", "renderRuntime", "Date", code + "\nreturn runtimePreflightSnapshot;")(
    state, api, () => Promise.resolve(), { now: () => clock }
  );
  run("p1");
  requests[0].reject(new Error("offline"));
  await drain();
  assert.equal(run("p1"), null, "unknown status must not show PASS");
  clock += 10001;
  run("p1");
  assert.equal(requests.length, 2);
  requests[1].resolve({ status: "WARN" });
  await drain();
  assert.equal(run("p1").status, "WARN");
});

test("GO uses async durable selection and guards an in-flight double click", async () => {
  const code = section(app, "async function goRuntime() {", "async function stopCueRuntime() {");
  const state = { project: { project_id: "p1" }, runtimeGoPending: false, runtimeJumpPending: false, runtimeUncertainCommand: null };
  const button = { disabled: false };
  let goRequests = 0, runtimeRequests = 0, updates = 0, finishGo, lastGoBody;
  const api = (url, opts) => {
    if (url.endsWith("/runtime")) {
      runtimeRequests++;
      return Promise.resolve({ current_cue: { cue_id: "cue1" } });
    }
    if (url.endsWith("/runtime/go")) {
      goRequests++;
      lastGoBody = opts.json;
      return new Promise((resolve) => { finishGo = resolve; });
    }
    throw new Error("Unexpected API " + url);
  };
  const go = new Function(
    "state", "api", "el", "requestID", "renderRuntime", "setMessage",
    "globalMessage", "errorMessage", "confirmPriorUncertainCueCommand", "rememberRuntimeUncertainCommand", code + "\nreturn goRuntime;"
  )(state, api, () => button, () => "idempotent-request", () => { updates++; return Promise.resolve(); },
    () => {}, {}, (e) => String(e), () => true, (command) => { state.runtimeUncertainCommand = command; });

  const first = go();
  const second = go();
  await drain();
  assert.equal(goRequests, 1);
  assert.equal(runtimeRequests, 1);
  assert.equal(button.disabled, true);
  assert.equal(lastGoBody.expected_current_cue_id, "cue1");
  assert.equal(lastGoBody.async, true);
  assert.equal(lastGoBody.request_id, "idempotent-request");
  finishGo({ result: { status: "ACCEPTED" } });
  await Promise.all([first, second]);
  assert.equal(updates, 1);
  assert.equal(state.runtimeGoPending, false);
  assert.equal(state.runtimeUncertainCommand, null, "accepted GO must clear uncertainty");
});

test("Preflight navigation paints an immediate checking state", async () => {
  const code = section(preflight, "navigate = async function stagecoreNavigate(page) {", "renderDashboard = async function stagecoreDashboard() {");
  const state = { project: { project_id: "p1", name: "Demo" }, page: "dashboard", preflightRenderSequence: 0 };
  const content = { innerHTML: "" };
  let resolveScan;
  const scan = () => new Promise((resolve) => { resolveScan = resolve; });
  const navigate = new Function(
    "state", "setPage", "setMessage", "globalMessage", "content", "renderPreflight",
    "stagecoreNavigateBase", "esc", "el", "errorMessage",
    "let navigate;\n" + code + "\nreturn navigate;"
  )(state, (page) => { state.page = page; }, () => {}, {}, content, scan,
    () => Promise.resolve(), (s) => s, () => null, String);
  const operation = navigate("preflight");
  assert.match(content.innerHTML, /Checking live readiness/);
  assert.equal(state.page, "preflight");
  resolveScan();
  await operation;
});

test("ordinary Runtime polls preserve mounted GO and selected Jump", () => {
  const code = section(app, "function patchActiveRuntimeView(markup) {",
    "async function renderRuntime(startPolling = false) {");
  const selectors = [
    ".page-head", ".cue-focus", ".stat-grid",
    ".runtime-running-executions", ".runtime-readiness-issues",
  ];
  const current = Object.fromEntries(selectors.map((name) => [name, { innerHTML: "old " + name }]));
  const upcoming = Object.fromEntries(selectors.map((name) => [name, { innerHTML: "new " + name }]));
  const go = { disabled: false, listener: "goRuntime" };
  const jump = { value: "cue-42" };
  const content = {
    querySelector: (selector) => current[selector] || null,
    // Deliberately fail if the helper attempts to remount the whole page.
    set innerHTML(_) { throw new Error("Full Runtime DOM replacement"); },
  };
  const document = {
    createElement: () => ({
      set innerHTML(value) { assert.equal(value, "<updated-runtime>"); },
      querySelector: (selector) => upcoming[selector] || null,
    }),
  };
  const patch = new Function("document", "content", code + "\nreturn patchActiveRuntimeView;")(
    document, content
  );
  assert.equal(patch("<updated-runtime>"), true);
  for (const selector of selectors) {
    assert.equal(current[selector].innerHTML, upcoming[selector].innerHTML);
  }
  assert.equal(go.listener, "goRuntime", "GO handler stays on its original button");
  assert.equal(jump.value, "cue-42", "Jump selection is not cleared by a poll");
  assert.equal(patch("<updated-runtime>"), true, "repeated polling stays in place");
});


test("unknown GO response survives a reload and blocks blind retry", async () => {
  const helper = section(app, "function loadRuntimeUncertainCommand() {", "function requestID() {");
  const goCode = section(app, "async function goRuntime() {", "async function stopCueRuntime() {");
  const saved = new Map();
  const sessionStorage = {
    getItem: (k) => saved.get(k) || null,
    setItem: (k, v) => saved.set(k, v),
    removeItem: (k) => saved.delete(k),
  };
  const key = "stagecore.runtime.uncertain_command.v1";
  const state = {
    project: { project_id: "p1" }, runtimeGoPending: false,
    runtimeJumpPending: false, runtimeUncertainCommand: null,
  };
  const helpers = new Function("sessionStorage", "state", "setMessage", "globalMessage", "confirm",
    "const runtimeUncertainCommandStorageKey = 'stagecore.runtime.uncertain_command.v1';\n" +
    helper + "\nreturn { load:loadRuntimeUncertainCommand, remember:rememberRuntimeUncertainCommand, verify:confirmPriorUncertainCueCommand };"
  )(sessionStorage, state, () => {}, {}, () => true);

  let postCount = 0;
  const messages = [];
  const api = (url) => {
    if (url.endsWith("/runtime")) return Promise.resolve({
      current_cue: { cue_id: "cue1", name: "Cue One" },
      next_cue: { cue_id: "cue2", name: "Cue Two" },
    });
    if (url.endsWith("/runtime/go")) {
      postCount++;
      return Promise.reject(new Error("Connection lost after write"));
    }
    throw new Error(url);
  };
  let allowNext = false;
  const go = new Function(
    "state", "api", "el", "requestID", "renderRuntime", "setMessage",
    "globalMessage", "errorMessage", "confirmPriorUncertainCueCommand", "rememberRuntimeUncertainCommand",
    goCode + "\nreturn goRuntime;"
  )(state, api, () => ({ disabled: false }), () => "request1",
    () => Promise.resolve(), (target, msg) => messages.push(msg), {}, (e) => String(e),
    (runtime, projectID) => {
      const prior = state.runtimeUncertainCommand;
      if (!prior) return true;
      assert.equal(projectID, "p1");
      if (!allowNext) return false;
      helpers.remember(null);
      return true;
    }, helpers.remember);

  await go();
  assert.equal(postCount, 1);
  assert.deepEqual(JSON.parse(saved.get(key)), { projectID: "p1", action: "GO" });
  assert.match(messages.at(-1), /may have reached the Hub/);
  assert.deepEqual(helpers.load(), { projectID: "p1", action: "GO" }, "reload must restore pending GO");

  // A fresh read alone MUST NOT silently send the following cue.
  await go();
  assert.equal(postCount, 1, "unverified command prevents a second GO");

  allowNext = true;
  await go();
  assert.equal(postCount, 2, "an explicit verification enables a NEW command");
});

test("a definite GO rejection clears pending ambiguity", async () => {
  const goCode = section(app, "async function goRuntime() {", "async function stopCueRuntime() {");
  const state = {
    project: { project_id: "p1" }, runtimeGoPending: false,
    runtimeJumpPending: false, runtimeUncertainCommand: null,
  };
  const go = new Function(
    "state", "api", "el", "requestID", "renderRuntime", "setMessage",
    "globalMessage", "errorMessage", "confirmPriorUncertainCueCommand", "rememberRuntimeUncertainCommand",
    goCode + "\nreturn goRuntime;"
  )(state, (url) => {
    if (url.endsWith("/runtime")) return Promise.resolve({ current_cue: null });
    return Promise.reject(Object.assign(new Error("definite rejection"), { status: 409 }));
  }, () => ({ disabled: false }), () => "request1",
  () => Promise.resolve(), () => {}, {}, (e) => String(e), () => true,
  (command) => { state.runtimeUncertainCommand = command; });
  await go();
  assert.equal(state.runtimeUncertainCommand, null);
});

test("a known acknowledged GO is not marked uncertain if UI refresh fails", async () => {
  const goCode = section(app, "async function goRuntime() {", "async function stopCueRuntime() {");
  const state = {
    project: { project_id: "p1" }, runtimeGoPending: false,
    runtimeJumpPending: false, runtimeUncertainCommand: null,
  };
  const go = new Function(
    "state", "api", "el", "requestID", "renderRuntime", "setMessage",
    "globalMessage", "errorMessage", "confirmPriorUncertainCueCommand", "rememberRuntimeUncertainCommand",
    goCode + "\nreturn goRuntime;"
  )(state, (url) => url.endsWith("/runtime")
    ? Promise.resolve({ current_cue: null })
    : Promise.resolve({ result: { status: "ACCEPTED" } }),
  () => ({ disabled: false }), () => "request1",
  () => Promise.reject(new Error("Runtime refresh unavailable")),
  () => {}, {}, (e) => String(e), () => true,
  (command) => { state.runtimeUncertainCommand = command; });
  await go();
  assert.equal(state.runtimeUncertainCommand, null);
});


test("JUMP and GO share an in-flight safety fence", async () => {
  const code = section(app, "async function jumpRuntime() {", "async function stopSessionRuntime() {");
  const state = {
    project: { project_id: "p1" }, runtimeGoPending: true,
    runtimeJumpPending: false, runtimeUncertainCommand: null,
  };
  let calls = 0;
  const controls = { jumpCueSelect: { value: "cue2" }, jumpButton: { disabled: false } };
  const jump = new Function(
    "state", "api", "el", "requestID", "renderRuntime", "setMessage",
    "globalMessage", "errorMessage", "confirmPriorUncertainCueCommand",
    "rememberRuntimeUncertainCommand", "confirm", code + "\nreturn jumpRuntime;"
  )(state, () => { calls++; return Promise.resolve({}); }, (id) => controls[id],
    () => "j1", () => Promise.resolve(), () => {}, {}, String,
    () => true, (command) => { state.runtimeUncertainCommand = command; }, () => true);
  await jump();
  assert.equal(calls, 0, "Jump cannot pass a pending GO");
  assert.equal(controls.jumpButton.disabled, false);

  state.runtimeGoPending = false;
  let complete;
  const api = (url) => {
    if (url.endsWith("/runtime")) return Promise.resolve({
      current_cue: { cue_id: "cue1" },
      cues: [{ cue_id: "cue2", name: "Next" }],
    });
    calls++;
    return new Promise((resolve) => { complete = resolve; });
  };
  const jump2 = new Function(
    "state", "api", "el", "requestID", "renderRuntime", "setMessage",
    "globalMessage", "errorMessage", "confirmPriorUncertainCueCommand",
    "rememberRuntimeUncertainCommand", "confirm", code + "\nreturn jumpRuntime;"
  )(state, api, (id) => controls[id], () => "j2",
    () => Promise.resolve(), () => {}, {}, String, () => true,
    (command) => { state.runtimeUncertainCommand = command; }, () => true);
  const first = jump2();
  const second = jump2();
  await drain();
  assert.equal(calls, 1, "double-click must not dispatch duplicate Jump");
  assert.equal(state.runtimeJumpPending, true);
  complete({ result: { status: "ACCEPTED" } });
  await Promise.all([first, second]);
  assert.equal(state.runtimeJumpPending, false);
  assert.equal(state.runtimeUncertainCommand, null);
});

test("unverified command from another Project cannot be silently cleared", () => {
  const helper = section(app, "function loadRuntimeUncertainCommand() {", "function requestID() {");
  const state = { runtimeUncertainCommand: { projectID: "show-A", action: "GO" } };
  let confirmCount = 0, warning = "";
  const verify = new Function("state", "setMessage", "globalMessage", "confirm",
    helper + "\nreturn confirmPriorUncertainCueCommand;"
  )(state, (_, message) => { warning = message; }, {}, () => { confirmCount++; return true; });
  assert.equal(verify({ current_cue: null, next_cue: null }, "show-B"), false);
  assert.equal(confirmCount, 0, "cross-Project bypass must not prompt to clear");
  assert.deepEqual(state.runtimeUncertainCommand, { projectID: "show-A", action: "GO" });
  assert.match(warning, /another Project/);
});


test("uncertain-command warning does not remount STOP or Blackout", () => {
  const code = section(app, "function patchActiveRuntimeView(markup) {",
    "async function renderRuntime(startPolling = false) {");
  const selectors = [
    ".page-head", ".cue-focus", ".stat-grid",
    ".runtime-running-executions", ".runtime-readiness-issues",
  ];
  const mounted = Object.fromEntries(selectors.map((s) => [s, { innerHTML: s }]));
  const stops = { stopButton: {}, emergencyBlackoutButton: {} };
  let warning = null;
  const header = mounted[".page-head"];
  header.before = (node) => { warning = node; };
  const content = {
    querySelector: (selector) => selector === ".runtime-unverified-command" ? warning : (mounted[selector] || null),
    set innerHTML(_) { throw new Error("Unsafe remount of live controls"); },
  };
  const document = {
    createElement: () => {
      let newWarning = null;
      const next = Object.fromEntries(selectors.map((s) => [s, { innerHTML: s }]));
      return {
        set innerHTML(markup) {
          if (markup.includes("WITH_WARNING")) {
            newWarning = { innerHTML: "Check previous GO", remove: () => { warning = null; } };
          }
        },
        querySelector: (selector) => selector === ".runtime-unverified-command" ? newWarning : (next[selector] || null),
      };
    },
  };
  const patch = new Function("document", "content", code + "\nreturn patchActiveRuntimeView;")(
    document, content
  );
  assert.equal(patch("WITH_WARNING"), true);
  assert.equal(warning?.innerHTML, "Check previous GO");
  assert.equal(patch("WITHOUT_WARNING"), true);
  assert.equal(warning, null);
  assert.ok(stops.stopButton);
  assert.ok(stops.emergencyBlackoutButton);
});


test("slow Runtime polls are serialized instead of starving live cue updates", async () => {
  const code = section(app, "function startRuntimePolling() {", "function stopRuntimePolling() {");
  const state = { page: "runtime", runtimeTimer: null, runtimePollingPending: false };
  const document = { hidden: false };
  let tick, calls = 0;
  const pending = [];
  const polling = new Function(
    "state", "document", "renderRuntime", "stopRuntimePolling", "setInterval",
    code + "\nreturn startRuntimePolling;"
  )(state, document,
    () => { calls++; return new Promise((resolve) => pending.push(resolve)); },
    () => {},
    (callback) => { tick = callback; return 1; }
  );
  polling();
  const first = tick();
  tick();
  tick();
  assert.equal(calls, 1, "slow GET must not spawn overlapping refreshes");
  assert.equal(state.runtimePollingPending, true);
  pending.shift()();
  await first;
  assert.equal(state.runtimePollingPending, false);
  const second = tick();
  assert.equal(calls, 2, "poll resumes immediately once the previous request finishes");
  pending.shift()();
  await second;
  document.hidden = true;
  await tick();
  assert.equal(calls, 2, "hidden page does not poll");
});
