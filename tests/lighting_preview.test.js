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
