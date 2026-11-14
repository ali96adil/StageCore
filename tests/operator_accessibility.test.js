"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const test = require("node:test");

const css = fs.readFileSync("internal/operatorweb/static/app.css", "utf8");
const cueStudioCSS = fs.readFileSync("internal/operatorweb/static/cue-studio.css", "utf8");
const app = fs.readFileSync("internal/operatorweb/static/app.js", "utf8");

test("interactive Operator controls keep a visible keyboard focus ring", () => {
  const rule = css.match(/:where\([^}]+\):focus-visible\s*\{([^}]+)\}/s);
  assert.ok(rule, "global focus-visible rule exists");
  assert.match(rule[1], /outline:\s*3px solid var\(--accent\)/);
  assert.match(rule[1], /outline-offset:\s*2px/);
});

test("Cue color remains supplemental to the visible Cue label text", () => {
  assert.match(app, /<td class="cue-label-cell" data-cue-color="\$\{cueColor\(cue\)\}">\$\{esc\(cue\.display_label \|\| "—"\)\}<\/td>/);
  assert.match(cueStudioCSS, /\.cue-label-cell\[data-cue-color="blue"\]\s*\{\s*box-shadow:\s*inset 0 -3px/);
  assert.match(cueStudioCSS, /\.cue-label-cell\[data-cue-color="green"\]/);
  assert.match(cueStudioCSS, /\.cue-label-cell\[data-cue-color="amber"\]/);
  assert.match(cueStudioCSS, /\.cue-label-cell\[data-cue-color="purple"\]/);
});
