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
        return [{ querySelector: (field) => ({
          value: field.endsWith("alias") ? "warm" : String(levels.warm),
          valueAsNumber: field.endsWith("alias") ? NaN : levels.warm,
        }) }];
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

test("Lighting Cue authoring rejects invalid levels and stale aliases with actionable errors", () => {
  const source = fs.readFileSync("internal/operatorweb/static/lighting-authoring.js", "utf8");
  const start = source.indexOf("function readOperation()");
  const end = source.indexOf("function lightingErrorMessage", start);
  const channelList = [{alias: "warm", minimum_level: 0, maximum_level: 80}];
  const run = ({command = "LIGHTING_CHANNELS_SET", alias = "warm", level = 55, fade = 0, selected = []} = {}) => {
    const rows = alias === null ? [] : [{
      querySelector(selector) {
        if (selector === ".lighting-level-alias") return {value: alias};
        if (selector === ".lighting-level-value") return {valueAsNumber: level};
        return null;
      },
    }];
    const document = {
      getElementById(id) {
        if (id === "lightingCueOperation") return {value: command};
        if (id === "lightingFadeMS") return {valueAsNumber: fade};
        return null;
      },
      querySelectorAll(selector) {
        if (selector === ".lighting-level-row") return rows;
        if (selector === ".lighting-blackout-node:checked") return selected.map((value) => ({value}));
        return [];
      },
    };
    const readOperation = vm.runInNewContext(source.slice(start, end) + "readOperation", {
      document, channels: () => channelList, tx: (key) => key,
    });
    return readOperation();
  };

  assert.deepEqual(JSON.parse(JSON.stringify(run())), {command_type: "LIGHTING_CHANNELS_SET", levels: {warm: 55}, fade_ms: 0});
  assert.throws(() => run({level: 125}), /levelInvalid/);
  assert.throws(() => run({level: NaN}), /levelInvalid/);
  assert.throws(() => run({alias: "stale"}), /channelUnavailable/);
  assert.throws(() => run({command: "LIGHTING_CHANNELS_FADE", level: 20, fade: 1.5}), /fadeInvalid/);
  assert.throws(() => run({command: "LIGHTING_BLACKOUT", fade: 600001, selected: ["node-a"]}), /blackoutFadeInvalid/);
  assert.throws(() => run({command: "LIGHTING_BLACKOUT", fade: 0}), /chooseNode/);

  const messageStart = source.indexOf("function lightingErrorMessage");
  const messageEnd = source.indexOf("async function saveCue", messageStart);
  const lightingErrorMessage = vm.runInNewContext(source.slice(messageStart, messageEnd) + "lightingErrorMessage", {
    tx: (key) => key, errorMessage: () => "fallback",
  });
  assert.equal(lightingErrorMessage({payload: {error: "LIGHTING_ALIAS_RESOLUTION_FAILED"}}), "cueInvalid");
  assert.equal(lightingErrorMessage({payload: {error: "LIGHTING_DEVICE_UNAVAILABLE"}}), "deviceUnavailable");
  assert.equal(lightingErrorMessage({payload: {error: "OTHER_ERROR"}}), "fallback");
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

test("editing a guided OSC action preserves unsupported parameter metadata", () => {
  const source = fs.readFileSync("internal/operatorweb/static/guided-ux.js", "utf8");
  const first = source.indexOf("function f002MIDIStatus");
  const last = source.indexOf("addActionEditor = function f002AddActionEditor", first);
  assert.ok(first >= 0 && last > first);
  const helpers = vm.runInNewContext(source.slice(first, last) + "({f002SyncActionCard})");
  const argumentControls = new Map();
  const argumentValue = {
    children: [],
    set innerHTML(_value) { this.children = []; },
    appendChild(child) { this.children.push(child); },
  };
  argumentControls.set(".f002-argument-type", {value: "", addEventListener() {}});
  argumentControls.set(".f002-argument-value", argumentValue);
  argumentControls.set(".f002-remove-argument", {addEventListener() {}});
  const argumentRow = {
    dataset: {},
    set innerHTML(_value) {},
    querySelector(selector) { return argumentControls.get(selector); },
  };
  const argumentEditor = vm.runInNewContext(
    source.slice(source.indexOf("function f002ArgumentRow"), source.indexOf("function f002EnhanceActionCard")) +
      "({f002ArgumentRow, f002ReadArgument})",
    {document: {createElement: (tag) => tag === "div" ? argumentRow : {}}}
  );
  const argument = argumentEditor.f002ArgumentRow({
    type: "string", value: "ready", vendor_argument: {index: 7},
  });
  argumentControls.set(".f002-argument-input", argumentValue.children[0]);
  const fields = new Map([
    [".action-timeout", {value: "{}"}],
    [".action-error", {value: "{}"}],
    [".f002-timeout-kind", {value: "none"}],
    [".f002-error-kind", {value: "CONTINUE"}],
    [".f002-action-kind", {value: "osc"}],
    [".action-capability", {value: "osc.send"}],
    [".action-parameters", {value: JSON.stringify({
      address: "/show/go",
      arguments: [{type: "string", value: "ready", vendor_argument: {index: 7}}],
      vendor_extension: {keep: true},
    })}],
    [".f002-action-delay", {value: "250"}],
    [".f002-osc-address", {value: "/show/go"}],
  ]);
  const card = {
    dataset: {f002Enhanced: "true"},
    querySelector(selector) { return fields.get(selector) || null; },
    querySelectorAll(selector) {
      assert.equal(selector, ".f002-argument-row");
      return [argument];
    },
  };

  helpers.f002SyncActionCard(card);

  const saved = JSON.parse(fields.get(".action-parameters").value);
  assert.deepEqual(JSON.parse(JSON.stringify(saved.vendor_extension)), {keep: true});
  assert.deepEqual(JSON.parse(JSON.stringify(saved.arguments[0].vendor_argument)), {index: 7});
  assert.equal(saved.arguments[0].value, "ready");
  assert.deepEqual(JSON.parse(JSON.stringify(argumentEditor.f002ReadArgument(argument).vendor_argument)), {index: 7});

  const midiFields = new Map([
    [".action-timeout", {value: "{}"}],
    [".action-error", {value: "{}"}],
    [".f002-timeout-kind", {value: "none"}],
    [".f002-error-kind", {value: "CONTINUE"}],
    [".f002-action-kind", {value: "midi"}],
    [".action-capability", {value: "midi.send"}],
    [".action-parameters", {value: JSON.stringify({
      destination_name: "IAC Driver Bus 1",
      bytes: [0x91, 60, 100],
      vendor_extension: {keep: true},
    })}],
    [".f002-action-delay", {value: "0"}],
    [".f002-midi-destination-mode", {value: "name"}],
    [".f002-midi-destination-name", {value: "IAC Driver Bus 1"}],
    [".f002-midi-destination", {value: "0"}],
    [".f002-midi-message", {value: "note_on"}],
    [".f002-midi-channel", {value: "2"}],
    [".f002-midi-data1", {value: "60"}],
    [".f002-midi-data2", {value: "100"}],
  ]);
  const midiCard = {
    dataset: {f002Enhanced: "true"},
    querySelector(selector) { return midiFields.get(selector) || null; },
    querySelectorAll(selector) {
      assert.equal(selector, ".f002-argument-row");
      return [];
    },
  };
  helpers.f002SyncActionCard(midiCard);
  const savedMIDI = JSON.parse(midiFields.get(".action-parameters").value);
  assert.deepEqual(JSON.parse(JSON.stringify(savedMIDI.vendor_extension)), {keep: true});
  assert.deepEqual(JSON.parse(JSON.stringify(savedMIDI.bytes)), [0x91, 60, 100]);
});

test("Cue Studio action clipboard crosses Projects only for the same editor session and warns about targets", () => {
  const source = fs.readFileSync("internal/operatorweb/static/cue-studio.js", "utf8");
  const first = source.indexOf("function serializeActionCopy");
  const last = source.indexOf("function duplicateCard", first);
  const state = { user: {}, project: { project_id: "project-a" }, f002Targets: [] };
  let targetWarning;
  let pasted;
  const context = {
    state, structuredClone,
    f002SyncActionCard() {},
    cardField: (card, field) => card[field],
    tr: (_arabic, english) => english,
    pick: () => null,
    globalMessage: {},
    setMessage: (_target, message, kind) => { targetWarning = {message, kind}; },
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
    ".action-error": { value: '{"retry_before_dispatch":true}' },
    ".action-mode": { value: "PARALLEL_BARRIER" },
    ".action-priority": { value: "P0" },
    ".action-enabled": { checked: false },
  };
  clipboard.copyActionToClipboard(card);
  clipboard.pasteClipboardAction();
  assert.equal(targetWarning.kind, "warn");
  assert.match(targetWarning.message, /target.*not available/i);
  assert.equal(pasted.action_id, "");
  assert.equal(pasted.target_ref, "VDMX");
  assert.equal(pasted.parameters.custom, true);
  assert.equal(pasted.timeout_policy.start_delay_ms, 120);
  assert.equal(pasted.error_policy.retry_before_dispatch, true);
  assert.equal(pasted.execution_mode, "PARALLEL_BARRIER");
  assert.equal(pasted.priority_class, "P0");
  assert.equal(pasted.enabled, false);
  pasted.parameters.custom = false;
  clipboard.pasteClipboardAction();
  assert.equal(pasted.parameters.custom, true);
  state.f002Targets = [{ logical_name: "VDMX" }];
  targetWarning = undefined;
  clipboard.copyActionToClipboard(card);
  clipboard.pasteClipboardAction();
  assert.equal(targetWarning, undefined, "configured targets should not raise an availability warning");
  pasted = undefined;
  state.project = { project_id: "project-b" };
  state.f002Targets = [];
  targetWarning = undefined;
  clipboard.pasteClipboardAction();
  assert.equal(pasted.target_ref, "VDMX", "copy keeps the authored logical target without importing device configuration");
  assert.equal(targetWarning.kind, "warn");
  assert.match(targetWarning.message, /not configured here.*remap/i);
  const copiedForDestination = pasted;
  clipboard.copyActionToClipboard(card);
  state.user = {};
  clipboard.pasteClipboardAction();
  assert.equal(pasted, copiedForDestination, "a different user session cannot read the previous clipboard");
});

test("Cue Studio copies selected Mixed-Cue actions in order as independent copies", () => {
  const source = fs.readFileSync("internal/operatorweb/static/cue-studio.js", "utf8");
  const first = source.indexOf("function serializeActionCopy");
  const last = source.indexOf("function duplicateCard", first);
  const state = {
    user: {}, project: { project_id: "project-a" },
    f002Targets: [{ logical_name: "LIGHTING" }, { logical_name: "VDMX" }],
  };
  const cards = [
    {
      selected: true,
      ".action-id": { value: "lighting-original" }, ".action-target": { value: "LIGHTING" },
      ".action-capability": { value: "lighting.channels.fade" },
      ".action-parameters": { value: '{"fade_ms":1200,"levels":{"warm":60}}' },
      ".action-timeout": { value: '{"timeout_ms":2000,"start_delay_ms":100}' },
      ".action-error": { value: '{"on_error":"FAIL_CUE"}' },
      ".action-mode": { value: "PARALLEL_BARRIER" }, ".action-priority": { value: "P0" },
      ".action-enabled": { checked: true },
      ".sc008-action-select": { checked: true },
    },
    {
      selected: true,
      ".action-id": { value: "osc-original" }, ".action-target": { value: "VDMX" },
      ".action-capability": { value: "osc.send" },
      ".action-parameters": { value: '{"address":"/scene/3","arguments":[3]}' },
      ".action-timeout": { value: '{"start_delay_ms":250}' },
      ".action-error": { value: '{"retry_before_dispatch":true}' },
      ".action-mode": { value: "SEQUENTIAL" }, ".action-priority": { value: "P1" },
      ".action-enabled": { checked: false },
      ".sc008-action-select": { checked: true },
    },
  ];
  for (const card of cards) card.querySelector = (selector) => selector === ".sc008-action-select" ? { checked: card.selected } : null;
  const inserted = [];
  let projectWarning;
  const context = {
    state, structuredClone, f002SyncActionCard() {},
    cardField: (card, field) => card[field],
    actionCards: () => cards,
    pick: () => null,
    globalMessage: {}, setMessage: (_target, message, kind) => { projectWarning = {message, kind}; }, tr: (_arabic, english) => english,
    addActionEditor: (action) => inserted.push(action), expandCard() {}, refreshActions() {},
  };
  const clipboard = vm.runInNewContext(
    "let actionClipboard = null; " + source.slice(first, last) +
    "({copySelectedActionsToClipboard, pasteClipboardAction})", context
  );
  clipboard.copySelectedActionsToClipboard();
  clipboard.pasteClipboardAction();
  assert.equal(inserted.length, 2);
  assert.deepEqual(inserted.map((action) => action.target_ref), ["LIGHTING", "VDMX"]);
  assert.deepEqual(inserted.map((action) => action.action_id), ["", ""]);
  assert.equal(inserted[0].parameters.levels.warm, 60);
  assert.equal(inserted[0].timeout_policy.start_delay_ms, 100);
  assert.equal(inserted[0].execution_mode, "PARALLEL_BARRIER");
  assert.equal(inserted[1].parameters.address, "/scene/3");
  assert.equal(inserted[1].timeout_policy.start_delay_ms, 250);
  assert.equal(inserted[1].error_policy.retry_before_dispatch, true);
  assert.equal(inserted[1].enabled, false);
  assert.equal(cards[0][".action-id"].value, "lighting-original");
  clipboard.pasteClipboardAction();
  assert.equal(inserted.length, 4);
  assert.notStrictEqual(inserted[0].parameters, inserted[2].parameters);
  inserted[2].parameters.levels.warm = 10;
  assert.equal(inserted[0].parameters.levels.warm, 60);
  state.project = { project_id: "project-b" };
  clipboard.pasteClipboardAction();
  assert.equal(inserted.length, 6, "selected Mixed-Cue actions can be pasted as independent Draft copies in another Project");
  assert.equal(projectWarning.kind, "warn");
  assert.match(projectWarning.message, /without copying device configuration.*verify each target/i);
  assert.deepEqual(inserted.slice(4).map((action) => action.action_id), ["", ""]);
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

test("Cue Studio history undoes and redoes mixed Action order, policies, links and unknown fields", () => {
  const source = fs.readFileSync("internal/operatorweb/static/cue-studio.js", "utf8");
  const first = source.indexOf("function createDraftHistory");
  const last = source.indexOf("function bindDraftHistory", first);
  const fields = new Map();
  let actions = [
    {action_id:"existing-osc", order_index:0, execution_mode:"PARALLEL_BARRIER", target_ref:"VDMX", capability_key:"osc.send", parameters:{address:"/cue/start", legacy_extension:{value:42}}, timeout_policy:{start_delay_ms:75, future_key:3}, error_policy:{on_error:"CONTINUE", retry_before_dispatch:true}, priority_class:"P1", enabled:true},
    {action_id:"existing-midi", order_index:1, execution_mode:"SEQUENTIAL", target_ref:"ABLETON", capability_key:"midi.send", parameters:{bytes:[144,60,100], legacy_mapping:{value:"stable"}}, timeout_policy:{timeout_ms:900}, error_policy:{on_error:"FAIL_CUE"}, priority_class:"P2", enabled:true},
  ];
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
  actions = [
    {action_id:"existing-midi", order_index:0, execution_mode:"PARALLEL", target_ref:"ABLETON", capability_key:"midi.send", parameters:{bytes:[144,64,88], legacy_mapping:{value:"edited"}}, timeout_policy:{timeout_ms:1100}, error_policy:{on_error:"CONTINUE"}, priority_class:"P2", enabled:false},
    {action_id:"new-action", order_index:1, execution_mode:"SEQUENTIAL", target_ref:"LIGHTING", capability_key:"lighting.channels.fade", parameters:{levels:{warm:35}, fade_ms:400}, timeout_policy:{start_delay_ms:20}, error_policy:{}, priority_class:"P1", enabled:true},
  ];
  fields.get("cueStartDelaySeconds").value = "0";
  fields.get("f018CueAt").value = "";
  draft.restoreDraftHistory(-1);
  assert.deepEqual(actions.map(action => action.action_id), ["existing-osc", "existing-midi"]);
  assert.deepEqual(actions.map(action => action.capability_key), ["osc.send", "midi.send"]);
  assert.equal(actions[0].execution_mode, "PARALLEL_BARRIER");
  assert.equal(actions[0].parameters.legacy_extension.value, 42);
  assert.equal(actions[0].timeout_policy.future_key, 3);
  assert.equal(actions[0].timeout_policy.start_delay_ms, 75);
  assert.equal(actions[0].error_policy.retry_before_dispatch, true);
  assert.equal(actions[1].parameters.legacy_mapping.value, "stable");
  assert.equal(actions[1].enabled, true);
  assert.equal(fields.get("cueStartDelaySeconds").value, "1.25");
  assert.equal(links[0].checked, true);
  assert.equal(fields.get("f018CueAt").value, "00:02:03:04");
  draft.restoreDraftHistory(1);
  assert.deepEqual(actions.map(action => action.action_id), ["existing-midi", "new-action"]);
  assert.deepEqual(actions.map(action => action.capability_key), ["midi.send", "lighting.channels.fade"]);
  assert.equal(actions[0].parameters.bytes[1], 64);
  assert.equal(actions[0].enabled, false);
  assert.equal(actions[1].timeout_policy.start_delay_ms, 20);
  assert.equal(fields.get("cueStartDelaySeconds").value, "0");
  assert.equal(links[0].checked, false);
  assert.equal(fields.get("f018CueAt").value, "");
});

test("Cue editor keyboard Undo/Redo shortcuts preserve native text-field history", () => {
  const source = fs.readFileSync("internal/operatorweb/static/cue-studio.js", "utf8");
  const start = source.indexOf("function bindDraftHistory");
  const end = source.indexOf("function bindEditorRecovery", start);
  const handlers = new Map();
  const elements = new Map();
  const deltas = [];
  const form = {dataset: {}, addEventListener() {}};
  const dialog = {open: true, addEventListener(type, handler) { handlers.set(type, handler); }};
  const anchor = {after(button) { button.isConnected = true; elements.set(button.id, button); }};
  const context = {
    form, anchor,
    pick(id) { return id === "cueDialog" ? dialog : id === "actionsEditor" ? {} : elements.get(id) || null; },
    document: {createElement() { return {
      isConnected: false, setAttribute(name, value) { this.attributes ??= {}; this.attributes[name] = value; },
    }; }},
    MutationObserver: class { observe() {} },
    tr: (_ar, en) => en,
    historyTimer: undefined,
    clearTimeout() {},
    setTimeout() { return 1; },
    readDraftHistory: () => "current draft",
    draftHistory: {reset() {}, current: () => "current draft", canMove: () => true},
    recordDraftHistory() {},
    refreshHistoryButtons() {},
    restoreDraftHistory: (delta) => deltas.push(delta),
  };
  vm.runInNewContext(source.slice(start, end) + "\nbindDraftHistory(form, anchor);", context);
  const dispatch = (values) => {
    let prevented = false;
    handlers.get("keydown")({...values, preventDefault() { prevented = true; }});
    return prevented;
  };
  assert.equal(dispatch({code:"KeyZ", metaKey:true, target:{tagName:"DIV"}}), true);
  assert.equal(dispatch({code:"KeyZ", ctrlKey:true, shiftKey:true, target:{tagName:"DIV"}}), true);
  assert.equal(dispatch({code:"KeyY", metaKey:true, target:{tagName:"BUTTON"}}), true);
  assert.equal(dispatch({code:"KeyZ", ctrlKey:true, target:{tagName:"TEXTAREA"}}), false);
  assert.equal(dispatch({code:"KeyZ", ctrlKey:true, altKey:true, target:{tagName:"DIV"}}), false);
  assert.deepEqual(deltas, [-1, 1, 1]);
  assert.match(elements.get("sc009UndoDraft").attributes["aria-keyshortcuts"], /Meta\+Z/);
  assert.match(elements.get("sc009RedoDraft").attributes["aria-keyshortcuts"], /Meta\+Shift\+Z/);
});

test("Cue Inspector dry-run plans saved actions and diagnoses invalid links without dispatch", () => {
  const source = fs.readFileSync("internal/operatorweb/static/cue-studio.js", "utf8");
  const start = source.indexOf("  function cueActionExecutionModeText(");
  const end = source.indexOf("  function cueInspector(", start);
  assert.ok(start >= 0 && end > start);
  const context = {
    tr: (_ar, en) => en,
    cuePolicyObject: (cue) => cue.execution_policy || {},
    cueLinkedIDs: (cue) => [...(cue.execution_policy?.linked_cue_ids || [])],
    api() { assert.fail("dry-run contacted an API"); },
  };
  const preview = vm.runInNewContext(source.slice(start, end) + "\n({planCueInspection, cueActionExecutionModeText})", context);
  const plan = preview.planCueInspection;
  const root = {
    cue_id: "root", enabled: true,
    execution_policy: { start_delay_ms: 120, linked_cue_ids: ["child", "missing", "disabled"] },
    actions: [{ order_index: 2, enabled: false }, { order_index: 1, execution_mode: "PARALLEL" }],
  };
  const child = { cue_id: "child", enabled: true, execution_policy: { linked_cue_ids: ["root"] } };
  const disabled = { cue_id: "disabled", enabled: false };
  const cues = [root, child, disabled];
  const result = plan(root, cues);
  assert.equal(result.group.map((item) => item.cue.cue_id).join(","), "root,child");
  assert.equal(result.group[0].delay, 120);
  assert.equal(result.group[0].actions.map((action) => action.order_index).join(","), "1,2");
  assert.equal(result.group[0].actions[1].enabled, false);
  assert.equal(result.linkedCuesConcurrent, true);
  assert.equal(root.actions[0].order_index, 2, "planning did not mutate the saved Cue");
  assert.equal(result.issues.map((issue) => issue.kind).join(","), "cycle,missing,disabled");
  assert.equal(plan(child, cues).parent, "root");
  const secondParent = { cue_id: "second", execution_policy: { linked_cue_ids: ["child"] } };
  const conflicting = [...cues, secondParent];
  const childPlan = plan(child, conflicting);
  assert.equal(childPlan.parent, undefined);
  assert.equal(childPlan.issues[0].kind, "multiple-parents");
  assert.equal(plan(root, conflicting).issues.find((issue) => issue.kind === "multiple-parents").id, "child");
  assert.equal(plan(disabled, cues).linkedCuesConcurrent, false);
  assert.match(preview.cueActionExecutionModeText("PARALLEL"), /joined at the next barrier or end of Cue/);
  assert.match(preview.cueActionExecutionModeText("PARALLEL_BARRIER"), /waits for all pending Actions/);
  assert.match(preview.cueActionExecutionModeText("SEQUENTIAL"), /earlier parallel Actions may continue/);
  assert.match(preview.cueActionExecutionModeText("unknown"), /Unsupported execution mode: unknown/);
});

test("Cue Inspector models linked Mixed Cue timing, barriers and assumed latency locally", () => {
  const source = fs.readFileSync("internal/operatorweb/static/cue-studio.js", "utf8");
  const start = source.indexOf("  function cueActionExecutionModeText(");
  const end = source.indexOf("  function cueInspector(", start);
  const context = {
    tr: (_ar, en) => en,
    cuePolicyObject: (cue) => cue.execution_policy || {},
    cueLinkedIDs: (cue) => [...(cue.execution_policy?.linked_cue_ids || [])],
    api() { assert.fail("local timing simulation contacted an API"); },
  };
  const {planCueInspection, cueInspectionTimeline} = vm.runInNewContext(
    source.slice(start, end) + "\n({planCueInspection, cueInspectionTimeline})", context
  );
  const root = {
    cue_id: "root", execution_policy: {start_delay_ms: 100, linked_cue_ids: ["child"]},
    actions: [
      {action_id:"a", order_index:1, execution_mode:"SEQUENTIAL", timeout_policy:{start_delay_ms:50,timeout_ms:500}},
      {action_id:"b", order_index:2, execution_mode:"PARALLEL", timeout_policy:{start_delay_ms:80,timeout_ms:500}},
      {action_id:"c", order_index:3, execution_mode:"PARALLEL_BARRIER", timeout_policy:{start_delay_ms:20,timeout_ms:500}},
      {action_id:"disabled", order_index:4, enabled:false, execution_mode:"SEQUENTIAL", timeout_policy:{start_delay_ms:999}},
      {action_id:"e", order_index:5, execution_mode:"SEQUENTIAL", timeout_policy:{start_delay_ms:10,timeout_ms:500}},
    ],
  };
  const child = {
    cue_id:"child", execution_policy:{start_delay_ms:300},
    actions:[{action_id:"child-a", order_index:1, execution_mode:"SEQUENTIAL", timeout_policy:{start_delay_ms:10}}],
  };
  const cues = [root, child];
  const plan = planCueInspection(root, cues);
  assert.equal(plan.linkedCuesConcurrent, true);
  const rows = cueInspectionTimeline(plan, {latencyMS:200});
  const byID = new Map(rows.map((row) => [row.action.action_id, row]));
  assert.equal(byID.get("a").dispatchMS, 150);
  assert.equal(byID.get("b").dispatchMS, 430, "PARALLEL starts after the prior SEQUENTIAL completion and its own delay");
  assert.equal(byID.get("c").dispatchMS, 370, "barrier action launches alongside earlier pending work");
  assert.deepEqual([...byID.get("e").waitFor], ["b", "c"]);
  assert.equal(byID.get("e").dispatchMS, 640, "later actions wait for all work at the barrier");
  assert.equal(byID.get("disabled").outcome, "SKIPPED_DISABLED");
  assert.equal(byID.get("child-a").dispatchMS, 310, "linked Cue delay is measured independently from GO");
  const lowerBound = cueInspectionTimeline(plan, {latencyMS:0});
  assert.equal(lowerBound.find((row) => row.action.action_id === "e").dispatchMS, 240);
  assert.match(source, /Local simulation only; no commands are sent/);
});

test("Cue Inspector fault scenarios identify affected outputs and timeout reason codes", () => {
  const source = fs.readFileSync("internal/operatorweb/static/cue-studio.js", "utf8");
  const start = source.indexOf("  function cueActionExecutionModeText(");
  const end = source.indexOf("  function cueInspector(", start);
  const context = {
    tr: (_ar, en) => en,
    cuePolicyObject: (cue) => cue.execution_policy || {},
    cueLinkedIDs: (cue) => [...(cue.execution_policy?.linked_cue_ids || [])],
    api() { assert.fail("fault scenario contacted an API"); },
  };
  const {planCueInspection, cueInspectionTimeline} = vm.runInNewContext(
    source.slice(start, end) + "\n({planCueInspection, cueInspectionTimeline})", context
  );
  const cue = {
    cue_id:"mixed", actions:[
      {action_id:"midi", order_index:1, target_ref:"LIVE", capability_key:"midi.send", execution_mode:"SEQUENTIAL"},
      {action_id:"light", order_index:2, target_ref:"FRONT", capability_key:"lighting.channels.fade", execution_mode:"SEQUENTIAL", parameters:{fade_ms:1200}, timeout_policy:{timeout_ms:100}},
      {action_id:"unknown", order_index:3, target_ref:"UNMAPPED", capability_key:"osc.send", execution_mode:"SEQUENTIAL"},
    ],
  };
  const plan = planCueInspection(cue, [cue]);
  const targets = [
    {logical_name:"LIVE", logical_type:"MACHINE_ROLE"},
    {logical_name:"FRONT", logical_type:"LIGHTING"},
  ];
  const offline = cueInspectionTimeline(plan, {scenario:"offline-target",offlineTarget:"LIVE",targets});
  assert.equal(offline[0].outcome, "SIMULATED_BLOCKED");
  assert.equal(offline[0].reason, "VIRTUAL_TARGET_OFFLINE");
  const companion = cueInspectionTimeline(plan, {scenario:"missing-companion",targets});
  assert.equal(companion[0].reason, "COMPANION_MISSING_FOR_MACHINE_ROLE");
  const missingMetadata = cueInspectionTimeline(plan, {scenario:"missing-companion",targets:[]});
  assert.equal(missingMetadata[0].reason, "TARGET_METADATA_UNAVAILABLE_COMPANION_IMPACT_UNKNOWN");
  const dmx = cueInspectionTimeline(plan, {scenario:"missing-dmx",targets});
  assert.equal(dmx[1].outcome, "SIMULATED_BLOCKED");
  assert.equal(dmx[1].reason, "DMX_NODE_MISSING");
  assert.equal(dmx[1].fadeMS, 1200);
  const timeout = cueInspectionTimeline(plan, {latencyMS:250,targets});
  assert.equal(timeout[1].outcome, "SIMULATED_TIMEOUT");
  assert.equal(timeout[1].reason, "ASSUMED_LATENCY_EXCEEDS_ACTION_TIMEOUT");
  assert.equal(timeout[1].completionMS, 100, "timeout bounds the simulated wait");
});

test("Cue Inspector failure scenarios follow fail-soft, strict and barrier behavior", () => {
  const source = fs.readFileSync("internal/operatorweb/static/cue-studio.js", "utf8");
  const start = source.indexOf("  function cueActionExecutionModeText(");
  const end = source.indexOf("  function cueInspector(", start);
  const context = {
    tr: (_ar, en) => en,
    cuePolicyObject: (cue) => cue.execution_policy || {},
    cueLinkedIDs: (cue) => [...(cue.execution_policy?.linked_cue_ids || [])],
    api() { assert.fail("failure simulation contacted an API"); },
  };
  const {planCueInspection, cueInspectionTimeline} = vm.runInNewContext(
    source.slice(start, end) + "\n({planCueInspection, cueInspectionTimeline})", context
  );
  const cue = {
    cue_id:"failure-path", actions:[
      {action_id:"offline-parallel", order_index:1, target_ref:"VIRTUAL-A", execution_mode:"PARALLEL"},
      {action_id:"continues", order_index:2, execution_mode:"SEQUENTIAL"},
      {action_id:"join", order_index:3, execution_mode:"PARALLEL_BARRIER"},
      {action_id:"after-join", order_index:4, execution_mode:"SEQUENTIAL"},
    ],
  };
  const plan = planCueInspection(cue, [cue]);
  const rows = cueInspectionTimeline(plan, {scenario:"offline-target",offlineTarget:"VIRTUAL-A"});
  assert.equal(rows[0].outcome, "SIMULATED_BLOCKED");
  assert.equal(rows[1].outcome, "PLANNED_ONLY", "async failures are not known before a barrier observes them");
  assert.equal(rows[3].outcome, "PLANNED_ONLY", "fail-soft errors continue after the barrier");
  const strictParallel = {
    cue_id:"strict-parallel", actions:[
      {action_id:"strict-async", order_index:1, target_ref:"VIRTUAL-A", execution_mode:"PARALLEL", error_policy:{on_error:"FAIL_CUE"}},
      {action_id:"strict-join", order_index:2, execution_mode:"PARALLEL_BARRIER"},
      {action_id:"after-strict-join", order_index:3, execution_mode:"SEQUENTIAL"},
    ],
  };
  const strictParallelRows = cueInspectionTimeline(planCueInspection(strictParallel, [strictParallel]), {
    scenario:"offline-target", offlineTarget:"VIRTUAL-A",
  });
  assert.equal(strictParallelRows[2].outcome, "SIMULATED_NOT_REACHED");
  assert.equal(strictParallelRows[2].dispatchMS, null);
  const strict = {
    cue_id:"strict", actions:[
      {action_id:"strict-failure", order_index:1, target_ref:"VIRTUAL-A", execution_mode:"SEQUENTIAL", error_policy:{on_error:"FAIL_CUE"}},
      {action_id:"strict-next", order_index:2, execution_mode:"SEQUENTIAL"},
    ],
  };
  const strictRows = cueInspectionTimeline(planCueInspection(strict, [strict]), {
    scenario:"offline-target", offlineTarget:"VIRTUAL-A",
  });
  assert.equal(strictRows[0].outcome, "SIMULATED_BLOCKED");
  assert.equal(strictRows[1].outcome, "SIMULATED_NOT_REACHED");
  assert.match(strictRows[1].reason, /ACTION_POLICY_FAIL_CUE:strict-failure/);
});
