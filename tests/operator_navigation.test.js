"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const test = require("node:test");

const app = fs.readFileSync("internal/operatorweb/static/app.js", "utf8");
const preflightSource = fs.readFileSync("internal/operatorweb/static/preflight.js", "utf8");
const preflightLoader = app.slice(app.indexOf("function loadProjectPreflight("), app.indexOf("function esc("));
const cueRenderer = app.slice(app.indexOf("async function renderCues("), app.indexOf("async function testCueFromWorkspace("));
const runtimeRenderer = app.slice(app.indexOf("function runtimeRenderIsCurrent("), app.indexOf("async function startRuntime("));
const preflightRenderer = preflightSource.slice(preflightSource.indexOf("async function renderPreflight()"), preflightSource.indexOf("navigate = async function"));
const openProject = app.slice(app.indexOf("async function openProject("), app.indexOf("function updateWorkspaceProject("));
const dashboardRenderer = app.slice(app.indexOf("async function renderDashboard("), app.indexOf("async function loadCues("));

function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return {promise, resolve, reject};
}

test("coalesces concurrent live Preflight reads for one project and session", async () => {
  const pending = deferred();
  let reads = 0;
  const user = {};
  const ctx = {
    state: {csrf: "token", user, project: {project_id: "project-a"}},
    projectPreflightRequests: new Map(),
    api() { reads++; return pending.promise; },
    encodeURIComponent,
    Date,
  };
  vm.runInNewContext(preflightLoader, ctx);
  const first = ctx.loadProjectPreflight("project-a");
  const second = ctx.loadProjectPreflight("project-a");
  assert.equal(reads, 1);
  const report = {status: "PASS"};
  pending.resolve(report);
  assert.equal(await first, report);
  assert.equal(await second, report);
  assert.equal(ctx.state.runtimePreflight, report);
  assert.ok(ctx.state.runtimePreflightCheckedAt > 0);
});

test("late Cue and Runtime replies cannot replace the selected project's Cues", async () => {
  const cues = deferred();
  const runtime = deferred();
  const user = {};
  const content = {innerHTML: "new project page"};
  const ctx = {
    state: {
      page: "cues", user, project: {project_id: "project-a"}, cueRenderGeneration: 0,
      cues: [{cue_id: "existing"}], currentRevision: {revision_id: "existing-revision"},
    },
    loadCues: () => cues.promise,
    api: () => runtime.promise,
    content,
    encodeURIComponent,
  };
  vm.runInNewContext(cueRenderer, ctx);
  const render = ctx.renderCues();
  ctx.state.project = {project_id: "project-b"};
  cues.resolve({cues: [{cue_id: "stale"}], revision: {revision_id: "stale-revision"}});
  runtime.resolve({cues: []});
  await render;
  assert.equal(ctx.state.cues[0].cue_id, "existing");
  assert.equal(ctx.state.currentRevision.revision_id, "existing-revision");
  assert.equal(content.innerHTML, "new project page");
});

test("late Cue reply cannot replace the page opened during the request", async () => {
  const cues = deferred();
  const runtime = deferred();
  const user = {};
  const content = {innerHTML: "Runtime page"};
  const ctx = {
    state: {page: "cues", user, project: {project_id: "project-a"}, cueRenderGeneration: 0, cues: [], currentRevision: null},
    loadCues: () => cues.promise,
    api: () => runtime.promise,
    content,
    encodeURIComponent,
  };
  vm.runInNewContext(cueRenderer, ctx);
  const render = ctx.renderCues();
  ctx.state.page = "runtime";
  cues.resolve({cues: [{cue_id: "late"}], revision: {revision_id: "late"}});
  runtime.resolve({cues: []});
  await render;
  assert.deepEqual(ctx.state.cues, []);
  assert.equal(ctx.state.currentRevision, null);
  assert.equal(content.innerHTML, "Runtime page");
});

test("late Preflight reply is ignored after navigating away", async () => {
  const reportRequest = deferred();
  const content = {innerHTML: "Cues page"};
  const user = {};
  const ctx = {
    state: {page: "preflight", user, project: {project_id: "project-a", name: "A"}, preflightRenderGeneration: 0},
    loadPreflight: () => reportRequest.promise,
    content,
  };
  vm.runInNewContext(preflightRenderer, ctx);
  const render = ctx.renderPreflight();
  ctx.state.page = "cues";
  reportRequest.resolve({status: "PASS", checks: []});
  await render;
  assert.equal(content.innerHTML, "Cues page");
});

test("the most recently selected project wins when project detail requests finish out of order", async () => {
  const replies = new Map();
  let navigations = 0;
  const ctx = {
    state: {projectOpenGeneration: 0, project: null},
    api(path) { return new Promise((resolve) => replies.set(path, resolve)); },
    encodeURIComponent,
    updateWorkspaceProject() {},
    navigate: async () => { navigations++; },
    setMessage() {},
    errorMessage: String,
  };
  vm.runInNewContext(openProject, ctx);
  const first = ctx.openProject("project-a");
  const second = ctx.openProject("project-b");
  replies.get("/api/v1/projects/project-b")({project: {project_id: "project-b"}});
  await second;
  replies.get("/api/v1/projects/project-a")({project: {project_id: "project-a"}});
  await first;
  assert.equal(ctx.state.project.project_id, "project-b");
  assert.equal(navigations, 1);
});

test("a project response arriving after sign-out cannot restore the prior workspace", async () => {
  let resolveProject;
  let navigations = 0;
  const user = {};
  const ctx = {
    state: {projectOpenGeneration: 0, project: null, user, csrf: "session"},
    api: () => new Promise((resolve) => { resolveProject = resolve; }),
    encodeURIComponent,
    updateWorkspaceProject() {},
    navigate: async () => { navigations++; },
    setMessage() {},
    errorMessage: String,
  };
  vm.runInNewContext(openProject, ctx);
  const pending = ctx.openProject("project-a");
  ctx.state.user = null;
  ctx.state.csrf = "";
  resolveProject({project: {project_id: "project-a"}});
  await pending;
  assert.equal(ctx.state.project, null);
  assert.equal(navigations, 0);
});

test("late Dashboard reply cannot replace a page opened during project navigation", async () => {
  const replies = new Map();
  const content = {innerHTML: "Preflight page"};
  const ctx = {
    state: {page: "dashboard", user: {}, project: {project_id: "project-a"}, dashboardRenderGeneration: 0},
    api(path) { return new Promise((resolve) => replies.set(path, resolve)); },
    encodeURIComponent,
    content,
  };
  vm.runInNewContext(dashboardRenderer, ctx);
  const render = ctx.renderDashboard();
  ctx.state.page = "preflight";
  replies.get("/api/v1/projects/project-a/dashboard")({project: {project_id: "wrong-project"}});
  replies.get("/api/v1/projects/project-a/runtime")(null);
  await render;
  assert.equal(ctx.state.project.project_id, "project-a");
  assert.equal(content.innerHTML, "Preflight page");
});

test("Preflight shows a clearly stale last report while the live check is pending", async () => {
  const liveReport = deferred();
  const cachedReport = {status: "PASS", evaluated_at: "earlier"};
  const user = {};
  const renders = [];
  const ctx = {
    state: {
      page: "preflight", user, project: {project_id: "project-a"}, preflightRenderGeneration: 0,
      runtimePreflightProjectID: "project-a", runtimePreflightUser: user, runtimePreflight: cachedReport,
    },
    loadPreflight: () => liveReport.promise,
    renderPreflightReport: (report, isCurrent) => renders.push({report, isCurrent}),
  };
  vm.runInNewContext(preflightRenderer, ctx);
  const render = ctx.renderPreflight();
  assert.deepEqual(renders, [{report: cachedReport, isCurrent: false}]);
  liveReport.resolve({status: "BLOCK", evaluated_at: "now"});
  await render;
  assert.equal(renders.length, 2);
  assert.equal(renders[1].isCurrent, true);
  assert.equal(renders[1].report.status, "BLOCK");
});

test("a failed live Preflight refresh labels the cached report stale rather than in progress", async () => {
  const user = {};
  const status = {textContent: ""};
  const ctx = {
    state: {
      page: "preflight", user, project: {project_id: "project-a"}, preflightRenderGeneration: 0,
      runtimePreflightProjectID: "project-a", runtimePreflightUser: user,
      runtimePreflight: {status: "PASS"},
    },
    loadPreflight: async () => { throw new Error("fixture unavailable"); },
    renderPreflightReport() {},
    el: (id) => id === "preflightRefreshStatus" ? status : null,
  };
  vm.runInNewContext(preflightRenderer, ctx);
  await assert.rejects(ctx.renderPreflight(), /fixture unavailable/);
  assert.match(status.textContent, /failed/);
  assert.match(status.textContent, /remains stale/);
});

test("Runtime renders usable controls before slow Preflight and keeps SHOW disabled until a fresh report arrives", async () => {
  const reportRequest = deferred();
  let preflightReads = 0;
  let runtimeReads = 0;
  const user = {role: "OPERATOR"};
  const content = {innerHTML: ""};
  const runtime = {
    project: {name: "Safe Test Project"}, mode: "EDIT", session: null,
    runtime_snapshot: {snapshot_version: 2, runtime_snapshot_id: "snapshot"},
    cues: [], current_cue: null, next_cue: null,
  };
  const ctx = {
    state: {
      csrf: "token", page: "runtime", user, project: {project_id: "project-a"},
      runtimeRenderGeneration: 0, runtimePreflightProjectID: "", runtimePreflightUser: null,
      runtimePreflight: null, runtimePreflightCheckedAt: 0, runtimePreflightRetryAfter: 0,
      runtimePreflightRequestGeneration: 0, runtimePreflightRefreshing: false,
      runtimePreflightRequestKey: "", runtimeForceExitAvailable: false,
      runtimeEndedRehearsal: null, runtimeCueReceipt: null,
    },
    content,
    api(path) {
      if (path.endsWith("/runtime")) { runtimeReads++; return Promise.resolve(runtime); }
      throw new Error(`Unexpected API request: ${path}`);
    },
    loadProjectPreflight() { preflightReads++; return reportRequest.promise.then((report) => {
      Object.assign(ctx.state, {
        runtimePreflightProjectID: "project-a", runtimePreflightUser: user,
        runtimePreflight: report, runtimePreflightCheckedAt: Date.now(),
      });
      return report;
    }); },
    document: {documentElement: {lang: "en"}},
    Date,
    encodeURIComponent,
    setShowSessionPresentation() {},
    esc: (value) => String(value ?? ""),
    fmtDate: () => "—",
    pill: (value) => `<span>${value}</span>`,
    cueParentMapFor: () => new Map(),
    cueInList: () => null,
    canRuntime: () => true,
    groupRuntimeReadinessIssues: () => [],
    runtimeReadinessGroupTitle: () => "",
    runtimeActiveExecutionMarkup: () => "",
    el: () => ({value: "PREVIEW", addEventListener() {}}),
    renderCueRelationship: () => "",
    stopCueRuntime() {},
    forceStopSessionRuntime() {},
    setInterval,
    clearInterval,
  };
  vm.runInNewContext(runtimeRenderer, ctx);
  await ctx.renderRuntime(false);
  assert.equal(runtimeReads, 1);
  assert.equal(preflightReads, 1);
  assert.match(content.innerHTML, /Safe Test Project/);
  assert.match(content.innerHTML, /CHECKING/);
  assert.match(content.innerHTML, /id="startShowButton"[^>]*disabled/);

  reportRequest.resolve({status: "PASS", checks: []});
  await new Promise((resolve) => setImmediate(resolve));
  await new Promise((resolve) => setImmediate(resolve));
  assert.ok(runtimeReads >= 2);
  assert.match(content.innerHTML, /Preflight: PASS/);
  assert.doesNotMatch(content.innerHTML, /id="startShowButton"[^>]*disabled/);

  await ctx.renderRuntime(false);
  assert.equal(preflightReads, 1, "fresh Preflight is not fetched again on Runtime refresh");
  ctx.state.runtimePreflightCheckedAt = Date.now() - 16000;
  await ctx.renderRuntime(false);
  assert.equal(preflightReads, 2, "an expired Preflight cache triggers one live refresh");
});
