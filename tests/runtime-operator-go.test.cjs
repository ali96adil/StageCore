"use strict";
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const { test } = require("node:test");

const source = fs.readFileSync("internal/operatorweb/static/app.js", "utf8");
const start = source.indexOf("async function goRuntime() {");
const end = source.indexOf("async function stopCueRuntime() {", start);
assert.ok(start >= 0 && end > start, "GO function available");

function makeHarness(post) {
  const calls = [];
  const messages = [];
  const state = {
    project: { project_id: "project-a" },
    runtimeGoFlight: false, runtimeGoUncertain: null, runtimeCanGo: true,
  };
  const button = { disabled: false, textContent: "GO" };
  const runtime = {
    session: { session_id: "session-1" },
    current_cue: { cue_id: "cue-1" },
    next_cue: { cue_id: "cue-2" },
    managed_output_blackout: false,
  };
  let generated = 0;
  const sandbox = {
    state,
    canRuntime: () => true,
    requestID: () => "same-uuid-" + ++generated,
    confirm: () => true,
    globalMessage: {},
    el: (id) => id === "goButton" ? button : null,
    setUncertainGO: (value) => { state.runtimeGoUncertain = value; },
    setMessage: (_target, message, kind) => { messages.push({ message, kind }); },
    errorMessage: (error) => error.message,
    renderRuntime: async () => {},
    api: async (path, options) => {
      if (path.endsWith("/runtime")) return runtime;
      assert.ok(path.endsWith("/runtime/go"));
      calls.push(options.json);
      return post(options.json, calls.length);
    },
  };
  const goRuntime = vm.runInNewContext(source.slice(start, end) + "\ngoRuntime;", sandbox);
  return { goRuntime, calls, messages, state, button, runtime, get generated() { return generated; } };
}

test("ambiguous acknowledgement retries the SAME GO identity and expected cue", async () => {
  const h = makeHarness(async (_json, count) => {
    if (count === 1) throw new Error("socket lost after POST");
    return { result: { status: "ACCEPTED" } };
  });
  await h.goRuntime();
  assert.equal(h.calls.length, 1);
  assert.ok(h.state.runtimeGoUncertain, "uncertain outcome retained");
  assert.match(h.messages.at(-1).message, /UNCONFIRMED/);
  h.runtime.current_cue.cue_id = "cue-2";
  await h.goRuntime();
  assert.equal(h.calls.length, 2);
  assert.equal(h.generated, 1);
  assert.equal(h.calls[0].request_id, h.calls[1].request_id);
  assert.equal(h.calls[1].expected_current_cue_id, "cue-1");
  assert.equal(h.state.runtimeGoUncertain, null);
});

test("a second click while GO is in flight never sends another POST", async () => {
  let release;
  const held = new Promise((resolve) => { release = resolve; });
  const h = makeHarness(async () => { await held; return { result: { status: "ACCEPTED" } }; });
  const first = h.goRuntime();
  const second = h.goRuntime();
  await second;
  release();
  await first;
  assert.equal(h.calls.length, 1);
  assert.equal(h.generated, 1);
});

test("an uncertain GO from a previous Session cannot advance this Session", async () => {
  const h = makeHarness(async () => ({ result: { status: "ACCEPTED" } }));
  h.state.runtimeGoUncertain = {
    projectID: "project-a", sessionID: "previous-session",
    requestID: "previous-uuid", expectedCurrentCueID: "previous-cue",
  };
  await h.goRuntime();
  assert.equal(h.calls.length, 0);
  assert.equal(h.state.runtimeGoUncertain.requestID, "previous-uuid");
});

test("GO does not dispatch while blackout is active", async () => {
  const h = makeHarness(async () => ({ result: { status: "ACCEPTED" } }));
  h.runtime.managed_output_blackout = true;
  await h.goRuntime();
  assert.equal(h.calls.length, 0);
});
