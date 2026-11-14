"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const test = require("node:test");

const source = fs.readFileSync("internal/operatorweb/static/cue-studio.js", "utf8");
const binder = source.slice(source.indexOf("  function bindActionPresets("), source.indexOf("  function formSignature("));

function node(tagName = "div") {
  return {
    tagName: tagName.toUpperCase(), children: [], listeners: {}, dataset: {}, isConnected: false,
    append(...items) { this.children.push(...items); },
    replaceChildren(...items) { this.children = items; },
    addEventListener(event, listener) { this.listeners[event] = listener; },
    setAttribute(name, value) { this[name] = value; },
    remove() { this.isConnected = false; },
  };
}

async function openPresets({ presets, targets = [] }) {
  const user = { user_id: "operator" };
  const dialog = node("dialog");
  dialog.open = true;
  const inserted = [];
  const workspace = node("section");
  workspace.append = (...items) => {
    workspace.children.push(...items);
    for (const item of items) item.isConnected = true;
  };
  const context = {
    state: { user, project: { project_id: "project" }, f002Targets: targets },
    workspace,
    document: { createElement: node },
    window: { confirm: () => true },
    pick: (id) => id === "cueDialog" ? dialog : null,
    tr: (_arabic, english) => english,
    api: async () => ({ presets }),
    errorMessage: String,
    addActionEditor(action) { inserted.push(action); },
    refreshActions() {},
  };
  await vm.runInNewContext(`${binder}\nbindActionPresets(workspace);`, context);
  const panel = workspace.children[0];
  panel.open = true;
  await panel.listeners.toggle();
  await new Promise(setImmediate);
  return { panel, inserted };
}

function find(root, tagName) {
  if (root?.tagName === tagName.toUpperCase()) return root;
  for (const child of root?.children || []) {
    const match = find(child, tagName);
    if (match) return match;
  }
  return null;
}

function contentText(root) {
  return [root?.textContent || "", ...(root?.children || []).map((child) => typeof child === "string" ? child : contentText(child))].join(" ");
}

test("Action preset preview exposes saved content and warns about a missing target", async () => {
  const action = {
    action_id: "stored-action-id", target_ref: "VDMX-MAIN", capability_key: "osc.send", execution_mode: "PARALLEL",
    parameters: { address: "/scene/7", arguments: [7] }, timeout_policy: { timeout_ms: 1500 },
  };
  const { panel, inserted } = await openPresets({ presets: [{ preset_id: "preset-a", name: "Scene 7", action }] });
  const list = find(panel, "ul");
  const row = list.children[0];
  const preview = find(row, "pre");
  assert.ok(preview, "the preset needs an explicit content preview");
  assert.match(preview.textContent, /\/scene\/7/);
  assert.match(contentText(row), /VDMX-MAIN/);
  assert.match(contentText(row), /Target is not present in this Project configuration/);
  const add = row.children.find((child) => child?.tagName === "BUTTON" && child.textContent === "Add to Cue");
  add.onclick();
  assert.equal(inserted.length, 1);
  assert.equal(inserted[0].action_id, "");
  assert.deepEqual(inserted[0].parameters, action.parameters);
  assert.equal(action.action_id, "stored-action-id", "insertion must not mutate the stored preset");
});

test("Action preset preview confirms a target present in the current Project", async () => {
  const { panel } = await openPresets({
    presets: [{ preset_id: "preset-a", name: "Scene 7", action: { target_ref: "VDMX-MAIN", capability_key: "osc.send" } }],
    targets: [{ logical_name: "VDMX-MAIN" }],
  });
  const row = find(panel, "ul").children[0];
  assert.match(contentText(row), /Target is available in this Project configuration/);
  assert.doesNotMatch(contentText(row), /Target is not present/);
});
