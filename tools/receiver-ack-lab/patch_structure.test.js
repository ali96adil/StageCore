"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const patch = JSON.parse(fs.readFileSync(
  path.join(__dirname, "StageCore-Live-Observer-LAB.maxpat"), "utf8",
)).patcher;
function texts() {
  return patch.boxes.map(({ box }) => String(box.text || ""));
}

test("maxpat is a safe non-playing observer patch with explicit start", () => {
  assert(Array.isArray(patch.boxes) && Array.isArray(patch.lines));
  const found = texts();
  assert(found.includes("live.thisdevice"));
  assert(found.includes("live.path live_set tracks 0"));
  assert(found.includes("live.observer playing_slot_index"));
  assert(found.includes("route playing_slot_index"));
  assert(found.includes("prepend stagecore_live_slot 0"));
  assert(found.includes("node.script max_receiver_bridge.js @autostart 0"));
  assert(found.includes("script start"));
  assert(found.includes("script stop"));
  // Documentation-only warning comments legitimately mention forbidden
  // object names. Inspect executable objects, not their cautionary text.
  const all = patch.boxes
    .filter(({ box }) => box.maxclass !== "comment")
    .map(({ box }) => String(box.text || ""))
    .join(" ");
  for (const forbidden of ["udpsend", "midiout", "dac~", "ezdac~", "live.object fire",
    "stagecore/go", "stagecore/stop", "autostart 1"]) {
    assert.equal(all.includes(forbidden), false,
      "unexpected production actuation in observer lab: " + forbidden);
  }
});

test("observer and node.script remain connected without mandatory send", () => {
  const edge = (from, to) => patch.lines.some(({ patchline }) =>
    patchline.source[0] === from && patchline.destination[0] === to);
  assert(edge("m4l", "defer"));
  assert(edge("defer", "path"));
  assert(edge("path", "observer"));
  assert(edge("observer", "route"));
  assert(edge("route", "pre"));
  assert(edge("pre", "node"));
  assert(edge("node", "print"));
  assert(edge("arm", "node"));
});
