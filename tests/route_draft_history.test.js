"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const test = require("node:test");

const configuration = fs.readFileSync("internal/operatorweb/static/configuration.js", "utf8");
const cueStudio = fs.readFileSync("internal/operatorweb/static/cue-studio.js", "utf8");
const localization = fs.readFileSync("internal/operatorweb/static/localization.js", "utf8");

test("Route Draft Undo and Redo are editor-gated, Draft-only, and use scoped history APIs", () => {
  assert.match(configuration, /if \(configurationEditable\(\)\)\s*\{[\s\S]*?configuration\.revision\?\.status === "DRAFT"[\s\S]*?route-draft-history/);
  assert.match(configuration, /id="routeHistoryUndo"[\s\S]*?model\.route_history\?\.can_undo/);
  assert.match(configuration, /id="routeHistoryRedo"[\s\S]*?model\.route_history\?\.can_redo/);
  assert.match(configuration, /method: "POST", json: \{ direction: "UNDO" \}/);
  assert.match(configuration, /method: "POST", json: \{ direction: "REDO" \}/);
  assert.match(localization, /"Undo routing change": "تراجع عن تعديل التوجيه"/);
  assert.match(localization, /"Redo routing change": "إعادة تعديل التوجيه"/);
});

test("Cue and Route persistent history advertise safe editor-level keyboard shortcuts", () => {
  assert.match(configuration, /aria-keyshortcuts="Control\+Z Meta\+Z"/);
  assert.match(configuration, /aria-keyshortcuts="Control\+Y Control\+Shift\+Z Meta\+Y Meta\+Shift\+Z"/);
  assert.match(configuration, /state\.page !== "configuration"[\s\S]*?target\?\.closest\?\.\("input,textarea,select,\[contenteditable='true'\]"\)/);
  assert.match(cueStudio, /undo\.setAttribute\("aria-keyshortcuts", "Control\+Z Meta\+Z"\)/);
  assert.match(cueStudio, /redo\.setAttribute\("aria-keyshortcuts", "Control\+Y Control\+Shift\+Z Meta\+Y Meta\+Shift\+Z"\)/);
  assert.match(cueStudio, /state\.page !== "cues"[\s\S]*?target\?\.closest\?\.\("input,textarea,select,\[contenteditable='true'\]"\)/);
});
