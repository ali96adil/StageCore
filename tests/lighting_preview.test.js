"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const test = require("node:test");

test("manual DMX preview supports continuous, timed and blackout without dispatch", () => {
  const controls = new Map();
  for (const name of ["Mode", "Duration", "Run", "Stop", "Result", "Raw"]) {
    controls.set("#lightingPreview" + name, {
      isConnected: true,
      addEventListener(event, fn) { this[event] = fn; },
    });
  }
  const get = (name) => controls.get("#lightingPreview" + name);
  const levels = { warm: 80 };
  let pending;
  const context = {
    window: {},
    document: {
      documentElement: { lang: "en" },
      getElementById(id) { return id === "lightingCueOperation" ? { value: "LIGHTING_CHANNELS_SET" } : null; },
      addEventListener() {},
      querySelectorAll(selector) {
        assert.equal(selector, ".lighting-level-row");
        return [{ querySelector: (field) => ({ value: field.endsWith("alias") ? "warm" : String(levels.warm) }) }];
      },
    },
    api() { assert.fail("preview attempted an API call"); },
    setTimeout(fn, ms) { pending = { fn, ms }; return pending; },
    clearTimeout(timer) { if (timer === pending) pending = undefined; },
  };
  const source = fs.readFileSync("internal/operatorweb/static/lighting-authoring.js", "utf8");
  vm.runInNewContext(source.replace(
    "  window.renderLightingCues = renderLightingCues;",
    `  controllerModel.nodes = [{channels: [{alias: "warm", channel_number: 3,
        minimum_level: 0, maximum_level: 60, inverted: true}]}];
       window.bindManualPreview = bindManualPreview;`
  ), context);
  context.window.bindManualPreview({ querySelector: (selector) => controls.get(selector) });

  get("Mode").value = "continuous";
  get("Run").click();
  assert.match(get("Result").textContent, /Simulated levels.*CH 3 = 102/);
  assert.equal(pending, undefined);
  get("Stop").click();
  assert.match(get("Result").textContent, /Simulated blackout.*CH 3 = 255/);

  get("Mode").value = "timed";
  get("Duration").valueAsNumber = 250;
  get("Run").click();
  assert.equal(pending.ms, 250);
  pending.fn();
  assert.match(get("Result").textContent, /Simulated blackout.*CH 3 = 255/);
  assert.equal(get("Stop").disabled, true);

  get("Duration").valueAsNumber = 600001;
  get("Run").click();
  assert.match(get("Result").textContent, /Enter an integer duration/);
  assert.equal(pending, undefined);

  get("Mode").value = "continuous";
  get("Raw").value = "1=20\n512=255";
  get("Run").click();
  assert.match(get("Result").textContent, /CH 1 = 20.*CH 512 = 255/);
  get("Stop").click();
  assert.match(get("Result").textContent, /CH 1 = 0.*CH 512 = 0/);
  get("Raw").value = "1=20\n001=255";
  get("Run").click();
  assert.match(get("Result").textContent, /Use unique channels/);
});

test("QLC+ draft preset produces one bounded OSC Float32 argument", () => {
  const source = fs.readFileSync("internal/operatorweb/static/guided-ux.js", "utf8");
  const first = source.indexOf("function f002QLCArgument");
  const last = source.indexOf("function f002ArgumentRow", first);
  const draft = vm.runInNewContext(source.slice(first, last) + "f002QLCArgument");
  for (const value of [0, 0.5, 1]) {
    assert.equal(JSON.stringify(draft(value)), JSON.stringify({type: "float32", value}));
  }
  for (const value of [-0.01, 1.01, "", NaN, Infinity]) {
    assert.throws(() => draft(value), /mapped float/);
  }
});

test("guided action options round-trip known fields and defer unknown policies to JSON", () => {
  const source = fs.readFileSync("internal/operatorweb/static/guided-ux.js", "utf8");
  const first = source.indexOf("function f002ParsePolicyObject");
  const last = source.indexOf("function f002ArgumentRow", first);
  const policies = vm.runInNewContext(source.slice(first, last) + "({f002ParseTimeoutPolicy, f002ParseErrorPolicy})");
  const readDelay = policies.f002ParseTimeoutPolicy('{"timeout_ms":300,"start_delay_ms":250}');
  assert.equal(readDelay.startDelayMS, 250);
  assert.equal(readDelay.timeoutMS, 300);
  assert.equal(policies.f002ParseTimeoutPolicy('{"start_delay_ms":null}'), null);
  assert.equal(policies.f002ParseTimeoutPolicy('{"unknown":1}'), null);
  const readError = policies.f002ParseErrorPolicy('{"retry_before_dispatch":true,"require_application_ack":true}');
  assert.equal(readError.retry, true);
  assert.equal(readError.requireACK, true);
  assert.equal(readError.onError, "CONTINUE");
  assert.equal(policies.f002ParseErrorPolicy('{"retry_before_dispatch":"yes"}'), null);
});

test("Cue Studio action clipboard stays in the current project and user session", () => {
  const source = fs.readFileSync("internal/operatorweb/static/cue-studio.js", "utf8");
  const first = source.indexOf("function serializeActionCopy");
  const last = source.indexOf("function duplicateCard", first);
  const state = { user: {}, project: { project_id: "project-a" } };
  let pasted;
  const context = {
    state, structuredClone,
    f002SyncActionCard() {},
    cardField: (card, field) => card[field],
    pick: () => null,
    addActionEditor: (action) => { pasted = action; },
    actionCards: () => [], refreshActions() {},
  };
  const clipboard = vm.runInNewContext(
    "let actionClipboard = null; " + source.slice(first, last) +
    "({copyActionToClipboard, pasteClipboardAction})", context
  );
  const card = {
    ".action-id": { value: "original-id" },
    ".action-target": { value: "VDMX" },
    ".action-capability": { value: "osc.send" },
    ".action-parameters": { value: '{"address":"/screen","custom":true}' },
    ".action-timeout": { value: '{"start_delay_ms":120}' },
  };
  clipboard.copyActionToClipboard(card);
  clipboard.pasteClipboardAction();
  assert.equal(pasted.action_id, "");
  assert.equal(pasted.target_ref, "VDMX");
  assert.equal(pasted.parameters.custom, true);
  assert.equal(pasted.timeout_policy.start_delay_ms, 120);
  pasted.parameters.custom = false;
  clipboard.pasteClipboardAction();
  assert.equal(pasted.parameters.custom, true);
  pasted = undefined;
  state.project = { project_id: "project-b" };
  clipboard.pasteClipboardAction();
  assert.equal(pasted, undefined);
  clipboard.copyActionToClipboard(card);
  state.user = {};
  clipboard.pasteClipboardAction();
  assert.equal(pasted, undefined);
});

test("raw preview values copy only through unique representable logical channels", () => {
  const source = fs.readFileSync("internal/operatorweb/static/lighting-authoring.js", "utf8");
  const first = source.indexOf("function previewDMXByte");
  const last = source.indexOf("function bindManualPreview", first);
  const {mapRawDMXToDraft, previewDMXByte, parseManualDMXRows} = vm.runInNewContext(
    source.slice(first, last) + "({mapRawDMXToDraft, previewDMXByte, parseManualDMXRows})", {tx: key => key}
  );
  const fixtures = [
    {alias:"warm", channel_number:1, minimum_level:0, maximum_level:100},
    {alias:"cold", channel_number:2, minimum_level:10, maximum_level:80, inverted:true}
  ];
  const mapped = mapRawDMXToDraft(parseManualDMXRows("1=128\n2=200"), fixtures);
  assert.equal(previewDMXByte(fixtures[0], mapped.get("warm")), 128);
  assert.equal(previewDMXByte(fixtures[1], mapped.get("cold")), 200);
  assert.throws(() => mapRawDMXToDraft(parseManualDMXRows("2=0"), fixtures), /previewUnmapped/);
  assert.throws(() => mapRawDMXToDraft(parseManualDMXRows("3=0"), fixtures), /previewUnmapped/);
  assert.throws(() => mapRawDMXToDraft(parseManualDMXRows("1=200"),
    [...fixtures, {...fixtures[0], alias:"other"}]), /previewUnmapped/);
  assert.throws(() => mapRawDMXToDraft(parseManualDMXRows("1=200"),
    [...fixtures, {...fixtures[0], channel_number:3}]), /previewUnmapped/);
});

test("Cue Studio draft history bounds checkpoints and discards redo on a new edit", () => {
  const source = fs.readFileSync("internal/operatorweb/static/cue-studio.js", "utf8");
  const first = source.indexOf("function createDraftHistory");
  const last = source.indexOf("const draftHistory", first);
  const history = vm.runInNewContext(source.slice(first, last) + "createDraftHistory(3)");
  history.reset("original");
  history.record("one");
  history.record(null);
  history.record("two");
  history.record("two");
  history.record("three");
  assert.equal(history.move(-1), "two");
  assert.equal(history.move(-1), "one");
  assert.equal(history.move(-1), undefined);
  assert.equal(history.move(1), "two");
  history.record("new branch");
  assert.equal(history.move(1), undefined);
  assert.equal(history.move(-1), "two");
});

test("Cue Studio history restores original action IDs and unknown Snapshot fields", () => {
  const source = fs.readFileSync("internal/operatorweb/static/cue-studio.js", "utf8");
  const first = source.indexOf("function createDraftHistory");
  const last = source.indexOf("function bindDraftHistory", first);
  const fields = new Map();
  let actions = [{action_id:"existing-id", execution_mode:"PARALLEL", parameters:{legacy_extension:{value:42}}, timeout_policy:{future_key:3}}];
  const links = [{value:"child", checked:true}];
  const context = {
    pick(id) {
      if (id === "linkedCuesEditor") return {querySelectorAll: selector => selector.endsWith(":checked") ? links.filter(x=>x.checked) : links};
      if (id === "actionsEditor") return {replaceChildren(){actions = [];}};
      if (!fields.has(id)) fields.set(id, {value:""});
      return fields.get(id);
    },
    actionCards: () => actions.map(action => ({
      action,
      querySelectorAll: () => [{value:action.execution_mode || "SEQUENTIAL"}],
      cloneNode() {
        const mode = {value:"SEQUENTIAL"};
        return {action:structuredClone(action), mode, querySelectorAll: () => [mode]};
      },
    })),
    cardField: card => ({value:card.action.action_id}),
    serializeActionCopy: card => ({...card.action, execution_mode:card.mode.value}),
    updateLinkedCuesEditorState(){}, refreshActions(){}, clearTimeout(){},
    addActionEditor: action => actions.push(action),
  };
  const draft = vm.runInNewContext(source.slice(first, last) + "({draftHistory, readDraftHistory, restoreDraftHistory})", context);
  fields.set("cueStartDelaySeconds", {value:"1.25"});
  fields.set("f018CueAt", {value:"00:02:03:04"});
  const baseline = draft.readDraftHistory();
  assert.notEqual(baseline, null);
  draft.draftHistory.reset(baseline);
  links[0].checked = false;
  actions = [{action_id:"replacement", parameters:{}}];
  fields.get("cueStartDelaySeconds").value = "0";
  fields.get("f018CueAt").value = "";
  draft.restoreDraftHistory(-1);
  assert.equal(actions[0].action_id, "existing-id");
  assert.equal(actions[0].execution_mode, "PARALLEL");
  assert.equal(actions[0].parameters.legacy_extension.value, 42);
  assert.equal(actions[0].timeout_policy.future_key, 3);
  assert.equal(fields.get("cueStartDelaySeconds").value, "1.25");
  assert.equal(links[0].checked, true);
  assert.equal(fields.get("f018CueAt").value, "00:02:03:04");
  draft.restoreDraftHistory(1);
  assert.equal(actions[0].action_id, "replacement");
});
