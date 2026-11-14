"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const test = require("node:test");

const source = fs.readFileSync("internal/operatorweb/static/security.js", "utf8");
const loadStart = source.indexOf("async function securityLoad()");
const loadEnd = source.indexOf("\nfunction securityRoleOptions", loadStart);
const loadFunction = source.slice(loadStart, loadEnd);
const levelsStart = source.indexOf("function securityProjectAccessLevelOptions(");
const levelsEnd = source.indexOf("\nasync function renderSecurity", levelsStart);
const levelsFunction = source.slice(levelsStart, levelsEnd);
const renderStart = source.indexOf("async function renderSecurity()");
const renderEnd = source.indexOf("\nfunction securityError", renderStart);
const renderFunction = source.slice(renderStart, renderEnd);

function basePayload(path) {
  if (path.endsWith("/secrets")) return {secrets: []};
  if (path.endsWith("/users")) return {users: []};
  if (path.includes("plugins/permissions")) return {permissions: []};
  if (path.includes("/audit")) return {records: []};
  if (path.endsWith("pairing/pending")) return {requests: []};
  if (path.endsWith("/companions")) return {companions: []};
  throw new Error(`unexpected API request: ${path}`);
}

async function renderSecurityHTML(api) {
  const content = {innerHTML: "", querySelectorAll: () => []};
  const element = {addEventListener() {}};
  const context = {
    api,
    state: {user: {role: "OWNER"}},
    content,
    el: () => element,
    esc: (value) => String(value ?? ""),
    fmtDate: (value) => String(value ?? ""),
    pill: (label) => `<span>${label}</span>`,
    securityRoleOptions: () => "",
  };
  await vm.runInNewContext(`${loadFunction}; ${levelsFunction}; ${renderFunction}; renderSecurity()`, context);
  return content.innerHTML;
}

async function renderSecurityHarness(api) {
  const elements = new Map();
  const listeners = new Map();
  const el = (id) => {
    if (!elements.has(id)) {
      const element = {
        value: "",
        disabled: false,
        textContent: "",
        addEventListener(eventName, handler) { listeners.set(`${id}:${eventName}`, handler); },
        set innerHTML(value) {
          this._innerHTML = value;
          const selected = [...String(value).matchAll(/<option value="([^"]*)"[^>]*selected/g)].at(-1);
          if (selected) this.value = selected[1];
        },
        get innerHTML() { return this._innerHTML || ""; },
      };
      elements.set(id, element);
    }
    return elements.get(id);
  };
  const content = {innerHTML: "", querySelectorAll: () => []};
  const context = {
    api,
    state: {user: {role: "OWNER"}},
    content,
    el,
    esc: (value) => String(value ?? ""),
    fmtDate: (value) => String(value ?? ""),
    pill: (label) => `<span>${label}</span>`,
    securityRoleOptions: () => "",
    confirm: () => true,
  };
  await vm.runInNewContext(`${loadFunction}; ${levelsFunction}; ${renderFunction}; renderSecurity()`, context);
  return {html: content.innerHTML, elements, listeners};
}

test("optional grant controls load only when qualification preview says not enforced", async () => {
  const calls = [];
  const api = async (path) => {
    calls.push(path);
    if (path === "/api/v1/project-access/cutover-preview") return {enforced: false, read_only: true};
    if (path === "/api/v1/projects") return {projects: [{project_id: "project-a", name: "Show A"}]};
    return basePayload(path);
  };
  const model = await vm.runInNewContext(`${loadFunction}; securityLoad()`, {api, state: {}});
  assert.equal(model.projectAccess.enforced, false);
  assert.equal(model.projectAccess.projects[0].project_id, "project-a");
  assert.ok(calls.includes("/api/v1/project-access/cutover-preview"));
  assert.ok(calls.includes("/api/v1/projects"));
});

test("missing optional ACL route leaves the existing Security workspace usable", async () => {
  const calls = [];
  const api = async (path) => {
    calls.push(path);
    if (path === "/api/v1/project-access/cutover-preview") {
      const error = new Error("HTTP 404");
      error.status = 404;
      throw error;
    }
    return basePayload(path);
  };
  const model = await vm.runInNewContext(`${loadFunction}; securityLoad()`, {api, state: {}});
  assert.equal(model.projectAccess, null);
  assert.equal(calls.includes("/api/v1/projects"), false);
  assert.equal(model.users.length, 0);
});

test("Security page hides grant controls unless the optional API confirms PREPARED_NOT_ENFORCED", async () => {
  const apiWithoutACL = async (path) => {
    if (path === "/api/v1/project-access/cutover-preview") {
      const error = new Error("HTTP 404");
      error.status = 404;
      throw error;
    }
    return basePayload(path);
  };
  const legacyHTML = await renderSecurityHTML(apiWithoutACL);
  assert.doesNotMatch(legacyHTML, /PROJECT ACCESS · QUALIFICATION ONLY/);

  const calls = [];
  const apiNotPrepared = async (path) => {
    calls.push(path);
    if (path === "/api/v1/project-access/cutover-preview") return {enforced: true};
    return basePayload(path);
  };
  const enforcedHTML = await renderSecurityHTML(apiNotPrepared);
  assert.doesNotMatch(enforcedHTML, /PROJECT ACCESS · QUALIFICATION ONLY/);
  assert.equal(calls.includes("/api/v1/projects"), false);

  const apiWithPreparedACL = async (path) => {
    if (path === "/api/v1/project-access/cutover-preview") return {enforced: false};
    if (path === "/api/v1/projects") return {projects: [{project_id: "show-1", name: "Show One"}]};
    if (path === "/api/v1/security/users") return {users: [{user_id: "operator-1", username: "Operator", role: "OPERATOR", enabled: true}]};
    return basePayload(path);
  };
  const preparedHTML = await renderSecurityHTML(apiWithPreparedACL);
  assert.match(preparedHTML, /PROJECT ACCESS · QUALIFICATION ONLY/);
  assert.match(preparedHTML, /PREPARED · NOT ENFORCED/);
  assert.match(preparedHTML, /Show One/);
  assert.match(preparedHTML, /Operator · OPERATOR/);
  assert.match(preparedHTML, /Save prepared access/);
});

test("Owner can load and save one prepared access level through the gated form", async () => {
  const calls = [];
  const api = async (path, options = {}) => {
    calls.push({path, options});
    if (path === "/api/v1/project-access/cutover-preview") return {enforced: false};
    if (path === "/api/v1/projects") return {projects: [{project_id: "show-1", name: "Show One"}]};
    if (path === "/api/v1/security/users") return {users: [{user_id: "operator-1", username: "Operator", role: "OPERATOR", enabled: true}]};
    if (path.endsWith("/access/users/operator-1") && options.method === "PUT") {
      return {access_level: "EDIT", permissions: ["project.view", "cue.edit"], enforced: false, phase: "PREPARED_NOT_ENFORCED"};
    }
    if (path.endsWith("/access/users/operator-1")) {
      return {access_level: "VIEW", permissions: ["project.view", "media.read"], enforced: false, phase: "PREPARED_NOT_ENFORCED"};
    }
    return basePayload(path);
  };
  const ui = await renderSecurityHarness(api);
  assert.match(ui.html, /PREPARED · NOT ENFORCED/);
  const project = ui.elements.get("projectAccessProject");
  const user = ui.elements.get("projectAccessUser");
  const level = ui.elements.get("projectAccessLevel");
  project.value = "show-1";
  user.value = "operator-1";
  await ui.listeners.get("loadProjectAccess:click")();
  assert.equal(level.value, "VIEW");
  assert.equal(ui.elements.get("saveProjectAccess").disabled, false);

  level.value = "EDIT";
  ui.listeners.get("projectAccessLevel:change")();
  await ui.listeners.get("projectAccessGrantForm:submit")({preventDefault() {}});
  const write = calls.find((call) => call.options.method === "PUT");
  assert.equal(write.path, "/api/v1/projects/show-1/access/users/operator-1");
  assert.deepEqual(JSON.parse(JSON.stringify(write.options.json)), {access_level: "EDIT"});
  assert.match(ui.elements.get("projectAccessCurrent").textContent, /Saved EDIT\. PREPARED_NOT_ENFORCED/);
});

test("a Project selection change while access is loading cannot enable a stale grant save", async () => {
  let resolveAccess;
  let writes = 0;
  const api = async (path, options = {}) => {
    if (path === "/api/v1/project-access/cutover-preview") return {enforced: false};
    if (path === "/api/v1/projects") return {projects: [
      {project_id: "show-a", name: "Show A"},
      {project_id: "show-b", name: "Show B"},
    ]};
    if (path === "/api/v1/security/users") return {users: [
      {user_id: "operator-a", username: "Operator A", role: "OPERATOR", enabled: true},
      {user_id: "operator-b", username: "Operator B", role: "OPERATOR", enabled: true},
    ]};
    if (options.method === "PUT") {
      writes++;
      return {access_level: "EDIT", permissions: ["cue.edit"], enforced: false, phase: "PREPARED_NOT_ENFORCED"};
    }
    if (path.includes("/access/users/")) {
      return new Promise((resolve) => { resolveAccess = resolve; });
    }
    return basePayload(path);
  };
  const ui = await renderSecurityHarness(api);
  const project = ui.elements.get("projectAccessProject");
  const user = ui.elements.get("projectAccessUser");
  const level = ui.elements.get("projectAccessLevel");
  const save = ui.elements.get("saveProjectAccess");
  project.value = "show-a";
  user.value = "operator-a";
  const loading = ui.listeners.get("loadProjectAccess:click")();

  project.value = "show-b";
  ui.listeners.get("projectAccessProject:change")();
  resolveAccess({access_level: "VIEW", permissions: ["project.view"], enforced: false, phase: "PREPARED_NOT_ENFORCED"});
  await loading;

  assert.equal(level.disabled, true);
  assert.equal(save.disabled, true);
  assert.match(ui.elements.get("projectAccessCurrent").textContent, /selection changed/i);
  await ui.listeners.get("projectAccessGrantForm:submit")({preventDefault() {}});
  assert.equal(writes, 0);
});

test("Viewer and disabled users only receive safe levels in the selector", () => {
  const viewer = vm.runInNewContext(`${levelsFunction}; securityProjectAccessLevelOptions("VIEWER", true, "VIEW")`);
  assert.match(viewer, /value="VIEW"/);
  assert.match(viewer, /value="VIEW" selected/);
  assert.equal((viewer.match(/ selected/g) || []).length, 1);
  assert.match(viewer, /value="NONE"/);
  assert.doesNotMatch(viewer, /value="OPERATE"/);
  assert.doesNotMatch(viewer, /value="MANAGE"/);

  const disabled = vm.runInNewContext(`${levelsFunction}; securityProjectAccessLevelOptions("OPERATOR", false, "OPERATE")`);
  assert.match(disabled, /value="OPERATE" selected disabled/);
  assert.match(disabled, /value="NONE"/);
  assert.doesNotMatch(disabled, /value="EDIT"/);

  const unknown = vm.runInNewContext(`${levelsFunction}; securityProjectAccessLevelOptions("UNKNOWN", true)`);
  assert.match(unknown, /value="NONE"/);
  assert.doesNotMatch(unknown, /value="OPERATE"/);

  const manage = vm.runInNewContext(`${levelsFunction}; securityProjectAccessLevelDescription("MANAGE")`);
  assert.match(manage, /Only the Owner can grant access or manage memberships/);
});

test("grant editor is visibly gated and confirms not-enforced writes", () => {
  assert.match(source, /model\.projectAccess\?\.enforced === false/);
  assert.match(source, /PREPARED · NOT ENFORCED/);
  assert.match(source, /not rely on them to restrict Project, Cue, Vault, device, or Show access/i);
  assert.match(source, /json: \{ access_level: level \}/);
  assert.match(source, /Store \$\{level\} access for this account\?/);
  assert.match(source, /result\.phase !== "PREPARED_NOT_ENFORCED"/);
});
