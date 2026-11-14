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

function openEditor({ storage, legacy = new Map(), base = "original", user = "owner", project = "project", cue = "cue", api, checkpointsAPI }) {
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
    api: async (path, options) => {
      if (path.endsWith("/editor-checkpoints")) {
        if (checkpointsAPI) return checkpointsAPI(path, options);
        if (!options) return { checkpoints: [] };
      }
      if (api) return api(path, options);
      throw new TypeError("offline");
    },
    setTimeout(fn) { timers.set(++serial, fn); return serial; },
    clearTimeout(id) { timers.delete(id); },
    form, workspace: element(), cue: { cue_id: cue },
  };
  vm.runInNewContext(binder + "\nbindEditorRecovery(form, cue, workspace);", ctx);
  const panel = nodes.get("sc009EditorRecovery");
  return {
    panel,
    local: panel.children.find((child) => child.id === "sc009LocalRestore"),
    discard: panel.children.find((child) => child.id === "sc009LocalDiscard"),
    restore: panel.children[1],
    review: panel.children.find((child) => child.id === "sc009LocalReview"),
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

test("account switch during a pending recovery write retains only the original account's draft", async () => {
  const storage = new Map();
  let deliver;
  const requests = [];
  const editor = openEditor({ storage, async api(_, options) {
    requests.push(options?.method || "GET");
    if (options?.method === "PUT") return new Promise(resolve => { deliver = resolve; });
    return { generation: "" };
  } });
  await settle();
  editor.edit("owner's unsent Cue");
  editor.flush();
  await settle();
  assert.equal(typeof deliver, "function");
  editor.switchUser("another account");
  deliver({ generation: "older-session-generation" });
  await settle();
  editor.saved();
  await settle();
  assert.deepEqual(requests, ["GET", "PUT"]);
  assert.equal(openEditor({ storage, user: "another account" }).local.hidden, true);
  const reopened = openEditor({ storage });
  reopened.local.onclick();
  assert.equal(reopened.value(), "owner's unsent Cue");
});

test("named checkpoint previews and restores to the editor without saving the Cue", async () => {
  const storage = new Map();
  const calls = [];
  const editor = openEditor({ storage, async api(path, options) {
    calls.push({ path, method: options?.method || "GET" });
    if (!options) return { generation: "" };
    return { generation: "" };
  }, async checkpointsAPI(path, options) {
    calls.push({ path, method: options?.method || "GET", json: options?.json });
    if (!options) return { checkpoints: [] };
    if (options.method === "POST") return {
      checkpoint_id: "checkpoint-1", cue_id: "cue", name: options.json.name,
      created_at: "2026-11-14T00:00:00Z", document: options.json.document,
    };
    if (options.method === "DELETE") return {};
    throw new Error("unexpected checkpoint request");
  } });
  await settle();
  const section = editor.panel.children.find((child) => child.id === "sc009NamedCheckpoints");
  assert.ok(section);
  const [name, save, list, preview, restore, remove] = section.children.slice(2);
  editor.edit("checkpoint rehearsal notes");
  name.value = "Rehearsal pass 1";
  await save.onclick();
  assert.equal(list.value, "checkpoint-1");
  assert.match(preview.value, /checkpoint rehearsal notes/);
  assert.equal(restore.hidden, false);
  editor.edit("later unsaved edit");
  restore.onclick();
  assert.equal(editor.value(), "checkpoint rehearsal notes");
  assert.equal(calls.some((call) => call.method === "PUT" || call.path.endsWith("/cues/cue") && call.method === "POST"), false,
    "restoring a checkpoint must not save the authored Cue");
  await remove.onclick();
  assert.equal(calls.some((call) => call.method === "DELETE" && call.path.endsWith("checkpoint-1")), true);
  await settle();
});

test("late checkpoint list cannot populate a different Project editor", async () => {
  const storage = new Map();
  let resolveList;
  const editor = openEditor({ storage, checkpointsAPI(_path, options) {
    if (!options) return new Promise((resolve) => { resolveList = resolve; });
    throw new Error("unexpected checkpoint write");
  } });
  await settle();
  editor.switchProject("different-project");
  resolveList({ checkpoints: [{ checkpoint_id: "stale", cue_id: "cue", name: "Stale", created_at: "now", document: { version: 1, cue_id: "cue", draft: draft("stale") } }] });
  await settle();
  const section = editor.panel.children.find((child) => child.id === "sc009NamedCheckpoints");
  assert.equal(section.children[4].children.length, 0);
});

test("an unsaved new Cue recovers by Project/User scope before it has a Cue ID", async () => {
  const storage = new Map();
  const local = openEditor({ storage, cue: "" });
  local.edit("new Cue saved in browser");
  local.close();
  const reopenedLocal = openEditor({ storage, cue: "" });
  assert.equal(reopenedLocal.local.hidden, false);
  reopenedLocal.local.onclick();
  assert.equal(reopenedLocal.value(), "new Cue saved in browser");
  assert.equal(openEditor({ storage, cue: "persisted-cue" }).local.hidden, true);

  const serverDraft = draft("new Cue saved on Hub");
  const reopenedServer = openEditor({ cue: "", async api(path) {
    assert.match(path, /cue-editor-recovery$/);
    return { generation: "new-cue-generation", document: {
      version: 1, cue_id: "", base: JSON.stringify(draft("original")), draft: serverDraft,
    } };
  } });
  await settle();
  assert.equal(reopenedServer.restore.hidden, false);
  reopenedServer.restore.onclick();
  assert.equal(reopenedServer.value(), "new Cue saved on Hub");
  assert.equal(reopenedServer.panel.children.find((child) => child.id === "sc009NamedCheckpoints").hidden, true,
    "named per-Cue checkpoints remain unavailable until the new Cue is persisted");
});
