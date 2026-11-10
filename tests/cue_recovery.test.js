"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const test = require("node:test");

const source = fs.readFileSync("internal/operatorweb/static/cue-studio.js", "utf8");
const binder = source.slice(source.indexOf("  function bindEditorRecovery("), source.indexOf("  function bindActionPresets("));
const draft = (value) => ({ fields: [value], links: [], actions: [] });
const settle = () => new Promise(setImmediate);

function openEditor({ storage, base = "original", user = "owner", project = "project", cue = "cue", api }) {
  const nodes = new Map();
  const timers = new Map();
  let current = draft(base);
  let serial = 0;
  function element() {
    return {
      open: true, isConnected: true, children: [],
      cloneNode() { return { ...element(), hidden: this.hidden }; },
      setAttribute() {},
      append(...children) { this.children.push(...children); },
      prepend(child) { nodes.set(child.id, child); },
      remove() { this.isConnected = false; },
      addEventListener(event, fn) { this[event] = fn; },
    };
  }
  const form = element();
  const dialog = element();
  nodes.set("cueDialog", dialog);
  nodes.set("actionsEditor", element());
  const ctx = {
    AbortController,
    MutationObserver: class { observe() {} disconnect() {} },
    state: { user: { user_id: user }, project: { project_id: project } },
    historyFields: ["name"],
    restoringDraft: false,
    readDraftHistory: () => JSON.stringify(current),
    applyEditorDraft: (value) => { current = value; },
    recordDraftHistory() {},
    pick: (id) => nodes.get(id),
    tr: (_, english) => english,
    document: { createElement: element },
    window: {
      addEventListener() {}, confirm: () => true,
      sessionStorage: {
        getItem: (key) => storage.get(key),
        setItem: (key, value) => storage.set(key, value),
        removeItem: (key) => storage.delete(key),
      },
    },
    api: api ?? (async () => { throw new TypeError("offline"); }),
    setTimeout(fn) { timers.set(++serial, fn); return serial; },
    clearTimeout(id) { timers.delete(id); },
    form, workspace: element(), cue: { cue_id: cue },
  };
  vm.runInNewContext(binder + "\nbindEditorRecovery(form, cue, workspace);", ctx);
  const panel = nodes.get("sc009EditorRecovery");
  return {
    local: panel.children.find((child) => child.id === "sc009LocalRestore"),
    discard: panel.children.find((child) => child.id === "sc009LocalDiscard"),
    review: panel.children.at(-1),
    value: () => current.fields[0],
    edit(value) { current = draft(value); form.input(); },
    flush() { for (const fn of timers.values()) fn(); timers.clear(); },
    saved() { dialog["stagecore:cue-saved"](); },
  };
}

test("offline edit survives reload, is private to the account/project/Cue, and clears after save", async () => {
  const storage = new Map();
  openEditor({ storage }).edit("offline notes");
  assert.equal(storage.size, 1);
  for (const options of [{ user: "other" }, { project: "other" }, { cue: "other" }]) {
    assert.equal(openEditor({ storage, ...options }).local.hidden, true);
  }
  const restored = openEditor({ storage });
  assert.equal(restored.local.hidden, false);
  restored.local.onclick();
  assert.equal(restored.value(), "offline notes");
  restored.saved();
  assert.equal(storage.size, 0);
  await settle();
});

test("a changed baseline preserves the offline draft for review without applying it", async () => {
  const storage = new Map();
  openEditor({ storage }).edit("unsynced edit");
  const stale = openEditor({ storage, base: "other tab already saved" });
  stale.local.onclick();
  assert.equal(stale.value(), "other tab already saved");
  assert.equal(stale.review.hidden, false);
  assert.match(stale.review.value, /unsynced edit/);
  stale.edit("new attempt before resolving");
  assert.match([...storage.values()][0], /unsynced edit/);
  await settle();
});

test("server generation conflict retains the local draft and never retries with a replacement generation", async () => {
  const storage = new Map();
  openEditor({ storage }).edit("local branch");
  const puts = [];
  const restored = openEditor({ storage, async api(_, options) {
    if (!options) return { generation: "" };
    puts.push(options.json);
    throw Object.assign(new Error("conflict"), { status: 409 });
  } });
  await settle();
  assert.equal(puts.length, 0);
  restored.local.onclick();
  restored.flush();
  await settle();
  restored.edit("local branch revised");
  restored.flush();
  await settle();
  assert.equal(puts.length, 1);
  assert.equal(puts[0].generation, "");
  assert.match([...storage.values()][0], /local branch revised/);
});
