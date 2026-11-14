"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const test = require("node:test");

function actionCard() {
  const fields = new Map([
    [".action-id", {value: ""}],
    [".action-target", {value: ""}],
    [".action-capability", {value: ""}],
    [".action-mode", {value: "SEQUENTIAL"}],
    [".action-priority", {value: "P1"}],
    [".action-enabled", {checked: true}],
    [".action-parameters", {value: "{}"}],
    [".action-timeout", {value: "{}"}],
    [".action-error", {value: "{}"}],
    [".action-title", {textContent: ""}],
    [".remove-action", {addEventListener() {}}],
  ]);
  return {
    fields,
    querySelector(selector) { return fields.get(selector) || null; },
  };
}

test("Cue Studio saves, reopens and previews a nested mixed compound Cue without changing action data", async () => {
  const app = fs.readFileSync("internal/operatorweb/static/app.js", "utf8");
  const editorStart = app.indexOf("function openCueEditor(cue) {");
  const editorEnd = app.indexOf('el("addActionButton").addEventListener', editorStart);
  const submitStart = app.indexOf('cueForm.addEventListener("submit", async (event) => {', editorEnd);
  const submitEnd = app.indexOf("\nfunction nextCueOrder()", submitStart);
  assert.ok(editorStart >= 0 && editorEnd > editorStart && submitStart > editorEnd && submitEnd > submitStart);

  const original = {
    cue_id: "cue-parent",
    display_label: "12A",
    name: "Opening compound cue",
    order_index: 12,
    cue_type: "STANDARD",
    criticality: "CRITICAL",
    enabled: true,
    notes_summary: "Keep the parent and child together",
    execution_policy: {
      linked_cue_ids: ["cue-child"],
      linked_cue_mode: "TOGETHER",
      start_delay_ms: 250,
      operator_cue_color: "#123456",
      future_group_policy: {preserve: true},
    },
    actions: [
      {
        action_id: "action-osc", order_index: 0, execution_mode: "PARALLEL_BARRIER",
        target_ref: "VDMX", capability_key: "osc.send",
        parameters: {address: "/show/open", arguments: [{type: "int32", value: 7, vendor_type_hint: "counter"}], vendor_extension: {version: 2}},
        timeout_policy: {timeout_ms: 2200, start_delay_ms: 40},
        error_policy: {on_error: "CONTINUE", require_application_ack: true},
        priority_class: "P1", enabled: true,
      },
      {
        action_id: "action-midi", order_index: 1, execution_mode: "SEQUENTIAL",
        target_ref: "ABLETON", capability_key: "midi.send",
        parameters: {destination_name: "IAC Driver Bus 1", bytes: [144, 60, 100], vendor_extension: {mapping: "opening"}},
        timeout_policy: {timeout_ms: 3000},
        error_policy: {on_error: "FAIL_CUE", retry_before_dispatch: true},
        priority_class: "P2", enabled: true,
      },
    ],
  };
  const child = {
    cue_id: "cue-child", display_label: "12B", name: "Lighting and Tablet",
    execution_policy: {linked_cue_ids: ["cue-grandchild"], linked_cue_mode: "TOGETHER", start_delay_ms: 90},
    actions: [
      {action_id: "action-light", order_index: 0, execution_mode: "PARALLEL", target_ref: "LIGHTING", capability_key: "lighting.channels.fade", parameters: {levels: {warm: 45}, fade_ms: 800}, timeout_policy: {start_delay_ms: 15}, error_policy: {on_error: "CONTINUE"}, priority_class: "P1", enabled: true},
      {action_id: "action-tablet", order_index: 1, execution_mode: "SEQUENTIAL", target_ref: "TABLET", capability_key: "tablet.media.play", parameters: {media_id: "clip-7"}, timeout_policy: {}, error_policy: {}, priority_class: "P1", enabled: true},
    ],
  };
  const grandchild = {
    cue_id: "cue-grandchild", display_label: "12C", name: "Final nested child",
    execution_policy: {linked_cue_mode: "TOGETHER", start_delay_ms: 120},
    actions: [{action_id: "action-laser", order_index: 0, execution_mode: "SEQUENTIAL", target_ref: "LASER", capability_key: "laser.safe_off", parameters: {device_id: "laser-1"}, timeout_policy: {}, error_policy: {}, priority_class: "P1", enabled: true}],
  };
  const state = {project: {project_id: "project-a"}, cues: [original, child, grandchild]};
  const fields = new Map([
    ["cueDialogTitle", {textContent: ""}],
    ["cueId", {value: ""}],
    ["cueLabel", {value: ""}],
    ["cueName", {value: ""}],
    ["cueColor", {value: "", dataset: {}}],
    ["cueCriticality", {value: ""}],
    ["cueEnabled", {checked: false}],
    ["cueExecutionPolicy", {value: ""}],
    ["cueStartDelaySeconds", {value: ""}],
    ["cueNotes", {value: ""}],
  ]);
  const cards = [];
  const actionsEditor = {
    set innerHTML(_value) { cards.length = 0; },
    appendChild(fragment) { cards.push(fragment.card); },
    querySelectorAll(selector) {
      assert.equal(selector, ".action-editor");
      return cards;
    },
  };
  const dialog = {showModal() {}, close() {}, dispatchEvent() {}};
  const template = {content: {cloneNode() {
    const card = actionCard();
    return {card, querySelector(selector) { return selector === ".action-editor" ? card : null; }};
  }}};
  const requests = [];
  let submit;
  const cueForm = {addEventListener(name, listener) { if (name === "submit") submit = listener; }};
  const context = {
    state, cueForm, actionsEditor, cueDialog: dialog,
    el(id) { return id === "actionTemplate" ? template : fields.get(id); },
    cueColor: () => "#123456",
    cuePolicyObject: (cue) => cue?.execution_policy || {},
    jsonText: (value) => JSON.stringify(value, null, 2),
    renderLinkedCuesEditor() {},
    linkedCuePolicyFromEditor: (policy) => ({...policy}),
    parseJSONField(text, label) {
      try { return JSON.parse(text || "{}"); }
      catch (_) { throw new Error(`${label} must contain valid JSON.`); }
    },
    cueByID(id) { return state.cues.find((cue) => cue.cue_id === id); },
    nextCueOrder: () => 13,
    async api(path, options) { requests.push({path, options}); return {}; },
    async renderCues() {},
    setMessage() {},
    errorMessage: String,
    globalMessage: {},
    Event,
  };
  vm.runInNewContext(
    app.slice(editorStart, editorEnd) + "\n" + app.slice(submitStart, submitEnd),
    context
  );
  assert.equal(typeof submit, "function");

  context.openCueEditor(original);
  assert.equal(cards.length, 2);
  assert.equal(fields.get("cueStartDelaySeconds").value, "0.25");
  await submit({preventDefault() {}});
  assert.equal(requests.length, 1);
  assert.equal(requests[0].options.method, "PUT");
  const saved = JSON.parse(JSON.stringify(requests[0].options.json));
  assert.deepEqual(saved.actions.map((action) => action.action_id), ["action-osc", "action-midi"]);
  assert.deepEqual(saved.actions.map((action) => action.order_index), [0, 1]);
  assert.equal(saved.actions[0].execution_mode, "PARALLEL_BARRIER");
  assert.equal(saved.actions[0].timeout_policy.start_delay_ms, 40);
  assert.equal(saved.actions[0].error_policy.require_application_ack, true);
  assert.deepEqual(saved.actions[0].parameters.vendor_extension, {version: 2});
  assert.deepEqual(saved.actions[0].parameters.arguments[0].vendor_type_hint, "counter");
  assert.deepEqual(saved.actions[1].parameters.vendor_extension, {mapping: "opening"});
  assert.deepEqual(saved.execution_policy.linked_cue_ids, ["cue-child"]);
  assert.equal(saved.execution_policy.start_delay_ms, 250);
  assert.equal(saved.execution_policy.future_group_policy.preserve, true);

  const reopened = {...original, ...saved};
  context.openCueEditor(reopened);
  assert.equal(cards.length, 2);
  assert.deepEqual(JSON.parse(cards[0].fields.get(".action-parameters").value).vendor_extension, {version: 2});
  assert.deepEqual(JSON.parse(cards[0].fields.get(".action-parameters").value).arguments[0].vendor_type_hint, "counter");
  assert.equal(cards[0].fields.get(".action-timeout").value.includes('"start_delay_ms": 40'), true);
  assert.equal(cards[1].fields.get(".action-target").value, "ABLETON");

  context.openCueEditor(null);
  fields.get("cueLabel").value = "13";
  fields.get("cueName").value = "New linked Mixed Cue";
  fields.get("cueColor").value = "#abcdef";
  fields.get("cueExecutionPolicy").value = JSON.stringify({
    linked_cue_ids: ["cue-child"], linked_cue_mode: "TOGETHER", future_group_policy: {preserve: true},
  });
  fields.get("cueStartDelaySeconds").value = "0.5";
  context.addActionEditor({
    target_ref: "LIGHTING", capability_key: "lighting.channels.fade", execution_mode: "PARALLEL",
    priority_class: "P1", enabled: true, parameters: {levels: {warm: 55}, fade_ms: 900},
    timeout_policy: {start_delay_ms: 25}, error_policy: {on_error: "CONTINUE"},
  });
  context.addActionEditor({
    target_ref: "TABLET", capability_key: "tablet.media.play", execution_mode: "SEQUENTIAL",
    priority_class: "P2", enabled: true, parameters: {media_id: "clip-9"}, timeout_policy: {}, error_policy: {},
  });
  await submit({preventDefault() {}});
  assert.equal(requests.length, 2);
  assert.equal(requests[1].options.method, "POST");
  const created = JSON.parse(JSON.stringify(requests[1].options.json));
  assert.equal(created.display_label, "13");
  assert.equal(created.name, "New linked Mixed Cue");
  assert.deepEqual(created.execution_policy.linked_cue_ids, ["cue-child"]);
  assert.equal(created.execution_policy.start_delay_ms, 500);
  assert.equal(created.execution_policy.operator_cue_color, "#abcdef");
  assert.equal(created.execution_policy.future_group_policy.preserve, true);
  assert.deepEqual(created.actions.map((action) => action.order_index), [0, 1]);
  assert.deepEqual(created.actions.map((action) => action.capability_key), ["lighting.channels.fade", "tablet.media.play"]);
  assert.deepEqual(created.actions.map((action) => action.action_id), [undefined, undefined]);
  assert.equal(created.actions[0].timeout_policy.start_delay_ms, 25);
  assert.equal(created.actions[1].parameters.media_id, "clip-9");

  const studio = fs.readFileSync("internal/operatorweb/static/cue-studio.js", "utf8");
  const planStart = studio.indexOf("  function cueActionExecutionModeText(");
  const planEnd = studio.indexOf("  function cueInspector(", planStart);
  const planner = vm.runInNewContext(studio.slice(planStart, planEnd) + "({planCueInspection})", {
    tr: (_ar, en) => en,
    cuePolicyObject: (cue) => cue.execution_policy || {},
    cueLinkedIDs: (cue) => [...(cue.execution_policy?.linked_cue_ids || [])],
  });
  const preview = planner.planCueInspection(reopened, [reopened, child, grandchild]);
  assert.deepEqual(JSON.parse(JSON.stringify(preview.group.map((member) => member.cue.cue_id))), ["cue-parent", "cue-child", "cue-grandchild"]);
  assert.deepEqual(JSON.parse(JSON.stringify(preview.group.map((member) => member.actions.length))), [2, 2, 1]);
  assert.equal(preview.group[0].delay, 250);
  assert.equal(preview.group[1].delay, 90);
  assert.equal(preview.linkedCuesConcurrent, true);

  const createdCue = {...created, cue_id: "cue-created"};
  const createdPreview = planner.planCueInspection(createdCue, [createdCue, child, grandchild]);
  assert.deepEqual(JSON.parse(JSON.stringify(createdPreview.group.map((member) => member.cue.cue_id))), ["cue-created", "cue-child", "cue-grandchild"]);
  context.openCueEditor(createdCue);
  assert.equal(fields.get("cueName").value, "New linked Mixed Cue");
  assert.equal(cards.length, 2);
  assert.equal(cards[0].fields.get(".action-id").value, "");
  assert.equal(cards[0].fields.get(".action-mode").value, "PARALLEL");
  assert.equal(JSON.parse(cards[0].fields.get(".action-parameters").value).levels.warm, 55);
  assert.equal(cards[1].fields.get(".action-target").value, "TABLET");
});
