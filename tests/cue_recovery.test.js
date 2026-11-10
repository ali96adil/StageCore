"use strict";

const assert = require("node:assert/strict");
const { webcrypto } = require("node:crypto");
const fs = require("node:fs");
const vm = require("node:vm");
const test = require("node:test");

const source = fs.readFileSync("internal/operatorweb/static/cue-studio.js", "utf8");
const binder = source.slice(source.indexOf("  function bindEditorRecovery("), source.indexOf("  function bindActionPresets("));
const draft = (value) => ({ fields: [value], links: [], actions: [] });
const settle = () => new Promise(setImmediate);

function openEditor({ storage, legacy = new Map(), base = "original", user = "owner", project = "project", cue = "cue", api }) {
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
  function webStorage(values) {
    return {
      get length() { return values.size; },
      key: (index) => [...values.keys()][index],
      getItem: (key) => values.get(key),
      setItem: (key, value) => values.set(key, value),
      removeItem: (key) => values.delete(key),
    };
  }
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
      crypto: webcrypto,
      sessionStorage: webStorage(legacy),
      localStorage: webStorage(storage),
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
    choices: panel.children[4],
    close() { dialog.open = false; dialog.close(); },
    value: () => current.fields[0],
    edit(value) { current = draft(value); form.input(); },
    flush() { for (const fn of timers.values()) fn(); timers.clear(); },
    saved() { dialog["stagecore:cue-saved"](); },
    switchProject(projectID) { ctx.state.project = { project_id: projectID }; },
    switchUser(userID) { ctx.state.user = { user_id: userID }; },
  };
}

test("offline edit survives a closed tab, is private to the account/project/Cue, and clears after save", async () => {
  const storage = new Map();
  const first = openEditor({ storage });
  first.edit("offline notes");
  first.close();
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
  assert.match([...storage.values()].join("\n"), /unsynced edit/);
  assert.match([...storage.values()].join("\n"), /new attempt before resolving/);
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

test("overlapping editors keep independent drafts and discard only the selected revision", () => {
  const storage = new Map();
  const first = openEditor({ storage });
  const second = openEditor({ storage });
  first.edit("first revision");
  second.edit("other tab");
  const review = openEditor({ storage });
  assert.equal(review.choices.hidden, false);
  first.edit("first tab new revision");
  review.discard.onclick();
  assert.deepEqual([...storage.values()].map((raw) => JSON.parse(raw).draft.fields[0]).sort(),
    ["first tab new revision", "other tab"]);
  const reopened = openEditor({ storage });
  reopened.choices.value = "1";
  reopened.choices.onchange();
  reopened.local.onclick();
  assert.equal(reopened.value(), "first tab new revision");
  reopened.saved();
  assert.equal(JSON.parse([...storage.values()][0]).draft.fields[0], "other tab");
});

test("existing tab recovery migrates to durable storage when restored", () => {
  const storage = new Map();
  const legacy = new Map([['stagecore.cue-recovery.v1:["owner","project","cue"]',
    JSON.stringify({ version: 1, cue_id: "cue", base: JSON.stringify(draft("original")), draft: draft("old local") })]]);
  const editor = openEditor({ storage, legacy });
  editor.local.onclick();
  assert.equal(editor.value(), "old local");
  assert.equal(legacy.size, 0);
  assert.equal(storage.size, 1);
});

test("a late recovery save cannot delete another Project's recovery on editor close", async () => {
  const storage = new Map();
  const requests = [];
  let resolveSave;
  const editor = openEditor({ storage, async api(path, options) {
    requests.push({ path, method: options?.method || "GET" });
    if (!options) return { generation: "" };
    if (options.method === "PUT") return new Promise(resolve => { resolveSave = resolve; });
    if (options.method === "DELETE") return { generation: "" };
    throw new Error("unexpected call");
  } });
  await settle();
  editor.edit("project one unsynced changes");
  editor.flush();
  await settle();
  assert.equal(typeof resolveSave, "function");
  editor.switchProject("project two");
  resolveSave({ generation: "old-project-generation" });
  await settle();
  editor.saved();
  await settle();
  assert.equal(requests.filter(r => r.method === "DELETE").length, 0,
    "an editor from another Project deleted the original Project recovery");
  assert.match([...storage.values()].join("\\n"), /project one unsynced changes/);
  const original = openEditor({ storage });
  assert.equal(original.local.hidden, false);
  original.local.onclick();
  assert.equal(original.value(), "project one unsynced changes");
});

test("offline drafts survive account switching and damaged unrelated recovery records", async () => {
  const storage = new Map();
  const original = openEditor({ storage });
  original.edit("unsynced recording notes");
  original.switchUser("different account");
  // A draft saved before the account switch remains isolated from the new account.
  const wrongAccount = openEditor({ storage, user: "different account" });
  assert.equal(wrongAccount.local.hidden, true);
  storage.set('stagecore.cue-recovery.v2:["owner","other project","cue"]:damaged', "{");
  const reopened = openEditor({ storage });
  assert.equal(reopened.local.hidden, false);
  reopened.local.onclick();
  assert.equal(reopened.value(), "unsynced recording notes");
  await settle();
});
