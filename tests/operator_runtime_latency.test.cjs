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
  const state = { project: { project_id: "p1" }, runtimeGoPending: false };
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
    "globalMessage", "errorMessage", code + "\nreturn goRuntime;"
  )(state, api, () => button, () => "idempotent-request", () => { updates++; return Promise.resolve(); },
    () => {}, {}, (e) => String(e));

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
