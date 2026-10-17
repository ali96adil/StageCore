"use strict";

// Exercise the exact pure ordering helper embedded in the offline Operator UI.
// The rest of app.js requires a browser DOM, so extract only this side-effect-free function.
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");

const app = fs.readFileSync(path.join(__dirname, "../internal/operatorweb/static/app.js"), "utf8");
const beginning = app.indexOf("function cueOrderIDsAfterMove(");
const ending = app.indexOf("\nasync function moveCueToPosition(", beginning);
assert.ok(beginning !== -1 && ending > beginning, "reorder helper must remain available");
const reorder = vm.runInNewContext(app.slice(beginning, ending) + "\ncueOrderIDsAfterMove;");
const cues = Array.from({ length: 50 }, (_, i) => ({
  cue_id: "cue-" + (i + 1),
  linked_cue_ids: i === 0 ? ["cue-2", "cue-3"] : [],
}));
function ids(result) { return Array.from(result); }

test("move Cue 50 to position 1 without losing Cue IDs or relationships", () => {
  const before = JSON.stringify(cues);
  const result = ids(reorder(cues, "cue-50", 1));
  assert.equal(result.length, 50);
  assert.equal(result[0], "cue-50");
  assert.equal(result[1], "cue-1");
  assert.equal(result[49], "cue-49");
  assert.equal(new Set(result).size, 50);
  assert.equal(JSON.stringify(cues), before, "never mutate loaded Cue objects");
});

test("move first Cue to last position", () => {
  const result = ids(reorder(cues, "cue-1", 50));
  assert.equal(result[0], "cue-2");
  assert.equal(result[49], "cue-1");
});

test("moving to the current position is a no-op", () => {
  assert.deepEqual(ids(reorder(cues, "cue-16", 16)), cues.map(c => c.cue_id));
});

test("reject invalid positions and unknown Cue IDs", () => {
  for (const position of [0, -1, 51, 2.5, NaN]) {
    assert.throws(() => reorder(cues, "cue-10", position));
  }
  assert.throws(() => reorder(cues, "not-a-cue", 10));
});

test("UI provides touch drag and a numeric fallback, only while editing a Draft", () => {
  assert.match(app, /installCueDragReorder\(\)/);
  assert.match(app, /touch.*mouse/);
  assert.match(app, /canModify && hasDraft && runtimeMode === "EDIT"/);
  assert.match(app, /cueMovePosition/);
  assert.match(app, /cue-drag-handle/);
  assert.match(app, /\/cues\/reorder/);
});
