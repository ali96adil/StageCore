"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const test = require("node:test");

const studio = fs.readFileSync("internal/operatorweb/static/cue-studio.js", "utf8");
const helperStart = studio.indexOf("  function setCueDraftHistoryButtonState(");
const helperEnd = studio.indexOf("\n  function enhanceCueList(", helperStart);
const historyHelper = studio.slice(helperStart, helperEnd);
const runtime = vm.createContext({});
vm.runInContext(historyHelper + "\nglobalThis.setCueDraftHistoryButtonState = setCueDraftHistoryButtonState;", runtime);

test("project Draft history buttons follow the actor-scoped persistent state", () => {
  const undo = {disabled: false};
  const redo = {disabled: false};
  runtime.setCueDraftHistoryButtonState(undo, redo, {can_undo: true, can_redo: false});
  assert.equal(undo.disabled, false);
  assert.equal(redo.disabled, true);
  runtime.setCueDraftHistoryButtonState(undo, redo, {can_undo: false, can_redo: true});
  assert.equal(undo.disabled, true);
  assert.equal(redo.disabled, false);
  runtime.setCueDraftHistoryButtonState(undo, redo, null);
  assert.equal(undo.disabled, true);
  assert.equal(redo.disabled, true);
});

test("project history controls are Draft-only and call the history API without execution routes", () => {
  assert.match(studio, /state\.currentRevision\?\.status === "DRAFT" && canEdit\(\)/);
  assert.match(studio, /api\(historyPath, \{method: "POST", json: \{direction\}\}\)/);
  assert.match(studio, /The Draft or Cue references changed/);
  const start = studio.indexOf("const historyPath");
  const end = studio.indexOf('const toolbar = document.createElement("section")', start);
  const historyControls = studio.slice(start, end);
  assert.doesNotMatch(historyControls, /runtime\/go|runtime\/jump|runtime\/start|publish/);
});
