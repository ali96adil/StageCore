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
