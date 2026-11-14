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

async function openPresets({ presets, targets = [], clipboard = [] }) {
  const user = { user_id: "operator" };
  const dialog = node("dialog");
  dialog.open = true;
  const inserted = [];
  const requests = [];
  const workspace = node("section");
  workspace.append = (...items) => {
    workspace.children.push(...items);
    for (const item of items) item.isConnected = true;
  };
  const context = {
    state: { user, project: { project_id: "project" }, f002Targets: targets },
    workspace,
    document: { createElement: node },
    structuredClone,
    window: { confirm: () => true },
    pick: (id) => id === "cueDialog" ? dialog : null,
    tr: (_arabic, english) => english,
    api: async (_path, request) => {
      if (request) requests.push(request);
      return { presets };
    },
    clipboardActions: () => clipboard,
    errorMessage: String,
    addActionEditor(action) { inserted.push(action); },
    refreshActions() {},
  };
  await vm.runInNewContext(`${binder}\nbindActionPresets(workspace);`, context);
  const panel = workspace.children[0];
  panel.open = true;
  await panel.listeners.toggle();
  await new Promise(setImmediate);
  return { panel, inserted, requests };
}

function find(root, tagName) {
  if (root?.tagName === tagName.toUpperCase()) return root;
  for (const child of root?.children || []) {
    const match = find(child, tagName);
    if (match) return match;
  }
  return null;
}

function findAll(root, tagName) {
  return [
    ...(root?.tagName === tagName.toUpperCase() ? [root] : []),
    ...(root?.children || []).flatMap((child) => typeof child === "string" ? [] : findAll(child, tagName)),
  ];
}

function contentText(root) {
  return [root?.textContent || "", ...(root?.children || []).map((child) => typeof child === "string" ? child : contentText(child))].join(" ");
}

test("Action preset preview exposes saved content and warns about a missing target", async () => {
  const action = {
    action_id: "stored-action-id", target_ref: "VDMX-MAIN", capability_key: "osc.send", execution_mode: "PARALLEL",
    parameters: { address: "/scene/7", arguments: [7] }, timeout_policy: { timeout_ms: 1500 },
  };
  const { panel, inserted } = await openPresets({ presets: [{ preset_id: "preset-a", name: "Scene 7", description: "Opening visual", action }] });
  const list = find(panel, "ul");
  const row = list.children[0];
  const preview = find(row, "pre");
  assert.ok(preview, "the preset needs an explicit content preview");
  assert.match(preview.textContent, /\/scene\/7/);
  assert.match(contentText(row), /VDMX-MAIN/);
  assert.match(contentText(row), /Opening visual/);
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

test("Multi-action Mixed-Cue preset inserts ordered independent copies", async () => {
  const actions = [
    { action_id: "saved-light", target_ref: "LIGHTING", capability_key: "lighting.channels.fade", parameters: { levels: { warm: 55 }, fade_ms: 900 } },
    { action_id: "saved-midi", target_ref: "ABLETON", capability_key: "midi.send", parameters: { status: 144, data1: 60, data2: 100 } },
    { action_id: "saved-osc", target_ref: "VDMX", capability_key: "osc.send", parameters: { address: "/scene/4", arguments: [4] } },
    { action_id: "saved-tablet", target_ref: "TABLET-MAIN", capability_key: "tablet.media.play", parameters: { media_id: "clip-7" } },
  ];
  const { panel, inserted } = await openPresets({
    presets: [{ preset_id: "mixed-preset", name: "Warm scene", actions }],
    targets: [{ logical_name: "LIGHTING" }, { logical_name: "ABLETON" }, { logical_name: "VDMX" }, { logical_name: "TABLET-MAIN" }],
  });
  const row = find(panel, "ul").children[0];
  assert.match(contentText(row), /4 actions/);
  assert.match(find(row, "pre").textContent, /\/scene\/4/);
  row.children.find((child) => child?.tagName === "BUTTON" && child.textContent === "Add 4 actions to Cue").onclick();
  assert.equal(inserted.length, 4);
  assert.deepEqual(inserted.map((action) => action.capability_key), ["lighting.channels.fade", "midi.send", "osc.send", "tablet.media.play"]);
  assert.deepEqual(inserted.map((action) => action.target_ref), ["LIGHTING", "ABLETON", "VDMX", "TABLET-MAIN"]);
  assert.deepEqual(inserted.map((action) => action.action_id), ["", "", "", ""]);
  assert.equal(inserted[0].parameters.levels.warm, 55);
  assert.equal(inserted[1].parameters.data1, 60);
  assert.equal(inserted[2].parameters.address, "/scene/4");
  assert.equal(inserted[3].parameters.media_id, "clip-7");
  assert.equal(actions[0].action_id, "saved-light");
  row.children.find((child) => child?.tagName === "BUTTON" && child.textContent === "Add 4 actions to Cue").onclick();
  assert.equal(inserted.length, 8);
  assert.notStrictEqual(inserted[0].parameters, inserted[4].parameters);
});

test("saving selected Mixed-Cue actions persists one ordered multi-action preset", async () => {
  const clipboard = [
    { action_id: "", target_ref: "LIGHTING", capability_key: "lighting.channels.fade", parameters: { fade_ms: 900 } },
    { action_id: "", target_ref: "VDMX", capability_key: "osc.send", parameters: { address: "/scene/4" } },
  ];
  const { panel, requests } = await openPresets({ presets: [], clipboard });
  const [name, description] = findAll(panel, "input");
  name.value = "Warm scene";
  description.value = "Opening wash and scene trigger";
  const save = panel.children.find((child) => child?.tagName === "BUTTON" && child.textContent === "Save copied action");
  await save.onclick();
  assert.deepEqual(JSON.parse(JSON.stringify(requests)), [
    { method: "POST", json: { name: "Warm scene", actions: clipboard, description: "Opening wash and scene trigger" } },
  ]);
});

test("Action preset search matches nested Mixed-Cue targets and payloads", async () => {
  const { panel } = await openPresets({ presets: [
    { preset_id: "warm", name: "Warm scene", description: "Entrance visual trigger", actions: [{ target_ref: "VDMX", capability_key: "osc.send", parameters: { address: "/scene/4" } }] },
    { preset_id: "fade", name: "Stage wash", description: "Blue lighting fade", action: { target_ref: "LIGHTING", capability_key: "lighting.channels.fade", parameters: { levels: { blue: 40 } } } },
  ] });
  const search = panel.children.find((child) => child?.className === "sc010-preset-search");
  assert.ok(search, "preset library needs a local search field");
  const rows = find(panel, "ul").children;
  search.value = "scene/4";
  search.listeners.input();
  assert.equal(rows[0].hidden, false);
  assert.equal(rows[1].hidden, true);
  search.value = "entrance visual";
  search.listeners.input();
  assert.equal(rows[0].hidden, false);
  assert.equal(rows[1].hidden, true);
  search.value = "blue";
  search.listeners.input();
  assert.equal(rows[0].hidden, true);
  assert.equal(rows[1].hidden, false);
});

test("Missing Mixed-Cue preset targets can be explicitly remapped per action", async () => {
  const actions = [
    { action_id: "saved-light", target_ref: "OLD-LIGHT", capability_key: "lighting.channels.fade", parameters: { fade_ms: 900 } },
    { action_id: "saved-osc", target_ref: "OLD-VISUAL", capability_key: "osc.send", parameters: { address: "/scene/4" } },
  ];
  const { panel, inserted } = await openPresets({
    presets: [{ preset_id: "mixed-preset", name: "Warm scene", actions }],
    targets: [{ logical_name: "LIGHTING" }, { logical_name: "VDMX" }],
  });
  const row = find(panel, "ul").children[0];
  const remapDetails = findAll(row, "details").find((item) => item.className === "sc010-preset-remaps");
  assert.ok(remapDetails, "missing targets should have an expandable remapping section");
  assert.equal(findAll(remapDetails, "select").length, 0, "large remapping controls should be lazy");
  remapDetails.open = true;
  remapDetails.listeners.toggle();
  const remaps = findAll(row, "select").filter((item) => item.className === "sc010-target-remap");
  assert.equal(remaps.length, 2);
  remaps[0].value = "LIGHTING";
  remaps[0].listeners.change();
  remaps[1].value = "VDMX";
  remaps[1].listeners.change();
  assert.match(contentText(row), /Targets will be remapped in this Draft Cue only/);
  remaps[0].value = "";
  remaps[0].listeners.change();
  assert.match(contentText(row), /Target is not present in this Project configuration/);
  remaps[0].value = "LIGHTING";
  remaps[0].listeners.change();
  row.children.find((child) => child?.tagName === "BUTTON" && child.textContent === "Add 2 actions to Cue").onclick();
  assert.deepEqual(inserted.map((action) => action.target_ref), ["LIGHTING", "VDMX"]);
  assert.deepEqual(inserted.map((action) => action.action_id), ["", ""]);
  assert.deepEqual(actions.map((action) => action.target_ref), ["OLD-LIGHT", "OLD-VISUAL"]);
});
