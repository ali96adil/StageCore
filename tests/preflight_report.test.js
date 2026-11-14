"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const test = require("node:test");

const source = fs.readFileSync("internal/operatorweb/static/preflight.js", "utf8");
const localization = fs.readFileSync("internal/operatorweb/static/localization.js", "utf8");

async function renderPreflightHarness(report) {
  const elements = new Map();
  const listeners = new Map();
  const downloaded = {clicked: false, blob: null, revoked: []};
  let apiReads = 0;
  const anchor = {
    style: {},
    click() { downloaded.clicked = true; },
    remove() {},
  };
  const content = {innerHTML: ""};
  const context = {
    navigate() {},
    renderDashboard() {},
    state: {project: {project_id: "show-a", name: "Qariyat Show"}},
    api: async () => { apiReads++; return report; },
    content,
    el(id) {
      if (!elements.has(id)) {
        elements.set(id, {addEventListener(event, handler) { listeners.set(`${id}:${event}`, handler); }});
      }
      return elements.get(id);
    },
    setPage() {},
    setMessage() {},
    errorMessage: (error) => String(error),
    esc: (value) => String(value ?? ""),
    fmtDate: (value) => String(value ?? ""),
    pill: (label) => `<span>${label}</span>`,
    document: {
      querySelectorAll: () => [],
      createElement: () => anchor,
      body: {appendChild() {}},
    },
    window: {setTimeout(callback) { callback(); return 1; }},
    URL: {
      createObjectURL(blob) { downloaded.blob = blob; return "blob:preflight-report"; },
      revokeObjectURL(url) { downloaded.revoked.push(url); },
    },
    Blob: class {
      constructor(parts, options) {
        this.parts = parts;
        this.type = options.type;
      }
    },
  };
  await vm.runInNewContext(`${source}\nrenderPreflight()`, context);
  return {content, downloaded, elements, listeners, anchor, get apiReads() { return apiReads; }};
}

test("Preflight downloads the exact evaluated report with a sanitized timestamped filename", async () => {
  const report = {
    status: "WARN",
    project_id: "show-a",
    runtime_snapshot_id: "snapshot-9",
    snapshot_version: 9,
    evaluated_at: "2026-11-14T02:00:00.000Z",
    checks: [{key: "midi.destination.action-a", category: "companion", status: "WARN", summary: "MIDI destination is present; application execution is unverified"}],
    roles: [],
    media: [],
    storage: {state: "HEALTHY", status: "PASS", free_bytes: 1024, reserve_bytes: 128, free_percent: 100, writable: true},
  };
  const ui = await renderPreflightHarness(report);

  assert.match(ui.content.innerHTML, /Download report/);
  assert.match(ui.content.innerHTML, /2026-11-14T02:00:00\.000Z/);
  const download = ui.listeners.get("exportPreflight:click");
  assert.equal(typeof download, "function");
  download();

  assert.equal(ui.downloaded.clicked, true);
  assert.equal(ui.anchor.href, "blob:preflight-report");
  assert.equal(ui.anchor.download, "StageCore-Preflight-qariyat-show-2026-11-14T02-00-00Z.json");
  assert.equal(ui.downloaded.blob.type, "application/json");
  assert.deepEqual(JSON.parse(ui.downloaded.blob.parts[0]), report);
  assert.deepEqual(ui.downloaded.revoked, ["blob:preflight-report"]);
});

test("Preflight report export uses the already loaded result without a second API read", async () => {
  const report = {
    status: "PASS", project_id: "show-a", evaluated_at: "2026-11-14T02:00:00Z",
    checks: [], roles: [], media: [], storage: {state: "HEALTHY", status: "PASS"},
  };
  const ui = await renderPreflightHarness(report);
  assert.equal(ui.apiReads, 1);
  ui.listeners.get("exportPreflight:click")();
  assert.equal(ui.apiReads, 1);
  assert.equal(JSON.parse(ui.downloaded.blob.parts[0]).evaluated_at, report.evaluated_at);
});

test("the report download control has an Arabic label", () => {
  assert.match(localization, /"Download report": "تنزيل التقرير"/);
  assert.ok(localization.includes("Run stagecore clock, then compare the Hub and RTC against a trusted local (LAN) time reference before rehearsal. If the Hub clock is outside the token validity window, local sign-in and session checks can fail."));
});
