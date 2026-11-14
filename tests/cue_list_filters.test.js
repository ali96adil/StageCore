"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const test = require("node:test");

const app = fs.readFileSync("internal/operatorweb/static/app.js", "utf8");
const filterSource = app.slice(app.indexOf("const cueListFilterStoragePrefix"), app.indexOf("async function renderCues("));
const cueHelpers = app.slice(app.indexOf("function cuePolicyObject("), app.indexOf("function cueParentMapFor("));

function makeView(cues, projectID = "show-a") {
  const values = new Map();
  const rows = [];
  const cueMap = new Map(cues.map((cue) => [cue.cue_id, cue]));
  const parents = new Map();
  for (const cue of cues) {
    for (const childID of cue.execution_policy?.linked_cue_ids || []) {
      if (!parents.has(childID)) parents.set(childID, cue.cue_id);
    }
  }
  for (const cue of cues) {
    rows.push({
      dataset: {
        cueId: cue.cue_id,
        search: "",
        actionTypes: "",
        mixed: "false",
        enabled: String(cue.enabled),
        linked: String(cue.execution_policy?.linked_cue_ids?.length > 0 || parents.has(cue.cue_id)),
      },
      hidden: false,
    });
  }

  const controls = {
    cueListSearch: { value: "" },
    cueListTypeFilter: { value: "all" },
    cueListEnabledFilter: { value: "all" },
    cueListLinkedFilter: { value: "all" },
    cueListEmptyResult: { hidden: true },
    cueListCount: { innerHTML: "" },
  };
  const storage = {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
  };
  const context = {
    state: { project: { project_id: projectID } },
    localStorage: storage,
    content: { querySelectorAll: () => rows },
    el: (id) => controls[id] || null,
  };
  const runtime = vm.createContext(context);
  vm.runInContext(cueHelpers + filterSource, runtime);
  rows.forEach((row, index) => {
    Object.assign(row.dataset, runtime.cueSearchRecord(cues[index], cueMap, parents));
    row.dataset.mixed = String(row.dataset.mixed);
    row.dataset.linked = String(row.dataset.linked);
  });
  return { context: runtime, controls, rows, values, cues };
}

const cues = [
  {
    cue_id: "parent", order_index: 1, display_label: "Q1", name: "Opening",
    notes_summary: "House opens", enabled: true, cue_type: "STANDARD",
    execution_policy: { linked_cue_ids: ["child"] },
    actions: [{ action_id: "light", capability_key: "lighting.front", target_ref: "HOUSE" }],
  },
  {
    cue_id: "child", order_index: 2, display_label: "Q1a", name: "Ableton entrance",
    notes_summary: "beat", enabled: false, cue_type: "STANDARD", execution_policy: { linked_cue_ids: ["grandchild"] },
    actions: [{ action_id: "midi", capability_key: "midi.send", target_ref: "ABLETON-IAC", parameters: { note: 60 } }],
  },
  {
    cue_id: "grandchild", order_index: 3, display_label: "Q1b", name: "Video fade",
    notes_summary: "slow fade", enabled: true, cue_type: "STANDARD", execution_policy: {},
    actions: [{ action_id: "osc-nested", capability_key: "osc.send", target_ref: "VDMX-MAIN" }],
  },
  {
    cue_id: "standalone", order_index: 4, display_label: "Q2", name: "Video",
    notes_summary: "screen", enabled: true, cue_type: "STANDARD", execution_policy: {},
    actions: [{ action_id: "osc", capability_key: "osc.send", target_ref: "VIDEO-MAIN" }],
  },
];

test("search includes labels, notes, targets and nested linked Actions", () => {
  const view = makeView(cues);
  view.controls.cueListSearch.value = "ableton-iac";
  view.context.applyCueListFilters();
  assert.deepEqual(view.rows.map((row) => !row.hidden), [true, true, false, false]);
});

test("Mixed filter sees compound action types through linked Cue chains", () => {
  const view = makeView(cues);
  view.controls.cueListTypeFilter.value = "mixed";
  view.context.applyCueListFilters();
  assert.deepEqual(view.rows.map((row) => !row.hidden), [true, true, false, false]);
});

test("state and linked filters combine without changing sequence row order", () => {
  const view = makeView(cues);
  const orderBefore = view.rows.map((row) => row.dataset.cueId);
  view.controls.cueListTypeFilter.value = "midi";
  view.controls.cueListEnabledFilter.value = "disabled";
  view.controls.cueListLinkedFilter.value = "linked";
  view.context.applyCueListFilters();
  assert.deepEqual(view.rows.map((row) => !row.hidden), [false, true, false, false]);
  assert.deepEqual(view.rows.map((row) => row.dataset.cueId), orderBefore);
});

test("saved filters are isolated by Project and malformed storage fails open", () => {
  const view = makeView(cues);
  view.controls.cueListSearch.value = "opening";
  view.context.applyCueListFilters();
  assert.equal(view.values.get("stagecore.cue-list.filters.v1:show-a").includes("opening"), true);
  view.context.state.project.project_id = "show-b";
  assert.equal(view.context.readCueListFilters("show-b").query, "");
  view.values.set("stagecore.cue-list.filters.v1:broken", "{");
  assert.equal(view.context.readCueListFilters("broken").type, "all");
});
