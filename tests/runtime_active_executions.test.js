"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const test = require("node:test");

const app = fs.readFileSync("internal/operatorweb/static/app.js", "utf8");
const start = app.indexOf("function runtimeActiveExecutionMarkup(");
const end = app.indexOf("async function renderRuntime(", start);
assert.notEqual(start, -1, "runtime active execution renderer exists");
const renderer = app.slice(start, end);

function render(runtime, cues = []) {
  const context = {
    cueInList: (items, id) => items.find((cue) => cue.cue_id === id) || null,
    cueDisplayName: (cue) => `${cue.display_label} · ${cue.name}`,
    esc: (value) => String(value ?? ""),
    fmtDate: (value) => `date:${value}`,
    pill: (label, kind) => `<span class="${kind}">${label}</span>`,
  };
  vm.runInNewContext(renderer, context);
  return context.runtimeActiveExecutionMarkup(runtime, cues);
}

test("runtime active execution panel is absent when there are no running Cues", () => {
  assert.equal(render({ active_executions: [], active_execution_count: 0 }), "");
});

test("runtime active execution panel lists each running Cue and execution ID", () => {
  const html = render({
    active_execution_count: 2,
    active_executions: [
      { cue_execution_id: "exec-a", cue_id: "cue-a", started_at: "2026-11-14T10:00:00Z" },
      { cue_execution_id: "exec-b", cue_id: "cue-b", started_at: "2026-11-14T10:00:01Z" },
    ],
  }, [
    { cue_id: "cue-a", display_label: "12", name: "Lighting" },
    { cue_id: "cue-b", display_label: "12.1", name: "Video" },
  ]);
  assert.match(html, /2 RUNNING/);
  assert.match(html, /12 · Lighting/);
  assert.match(html, /12\.1 · Video/);
  assert.match(html, /exec-a/);
  assert.match(html, /exec-b/);
  assert.equal((html.match(/class="runtime-active-execution"/g) || []).length, 2);
});

test("operator STOP controls say STOP ALL CUES when targeting every active execution", () => {
  assert.match(app, /id="stopCueButton"[^>]*>STOP ALL CUES/);
  assert.match(app, /id="cueCheckStopButton"[^>]*>STOP ALL CUES/);
  assert.match(app, /STOP ALL CUES interrupts every active Cue in this Session/);
  assert.doesNotMatch(app, /STOP CUE interrupts ALL active Cues/);
});
