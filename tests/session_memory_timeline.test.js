"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const test = require("node:test");

const source = fs.readFileSync("internal/operatorweb/static/memory.js", "utf8");
const helpers = source.slice(source.indexOf("const sessionEventLabels ="), source.indexOf("async function renderRehearsalApproval("));

function harness() {
  const context = {
    esc: (value) => String(value).replaceAll("&", "&amp;").replaceAll("<", "&lt;"),
    fmtDate: String,
    pill: (label) => `<pill>${label}</pill>`,
  };
  vm.runInNewContext(helpers, context);
  return context;
}

test("Session timeline filters by event category and renders only projected fields", () => {
  const h = harness();
  const events = [
    {event_id: "safety-1", event_type: "runtime.emergency_blackout.applied", category: "safety", priority: "P0", occurred_at: "now", correlation_id: "corr-safe"},
    {event_id: "device-1", event_type: "stage_device.command.failed", category: "device", priority: "P1", occurred_at: "later", warning_count: 1, warning_kinds: ["preflight"]},
  ];
  const safety = h.sessionEventTimelineMarkup(events, "safety");
  assert.match(safety, /Emergency Blackout applied/);
  assert.doesNotMatch(safety, /Device command failed/);
  assert.match(h.sessionEventTimelineMarkup(events, "device"), /Preflight warning/);
  assert.match(h.sessionEventTimelineMarkup([], "all"), /No runtime events match/);
});

test("Session JSON export keeps execution evidence but excludes free-form adapter response text", () => {
  const h = harness();
  const projection = h.sessionTraceExportProjection(
    {session_id: "s1", project_id: "p1", session_type: "REHEARSAL", name: "Dress", runtime_snapshot_id: "snap", status: "COMPLETED"},
    [{cue_execution_id: "c1", cue_id: "cue1", name: "Opening", result: "COMPLETED", actions: [{action_execution_id: "a1", action_id: "a", target_ref: "video", capability_key: "osc.send", result: "COMPLETED", ack_level: "TRANSPORT_ONLY", delivery_evidence: "EXTERNAL_EXECUTION_UNKNOWN", response_summary: "untrusted private endpoint response"}]}],
    [{event_id: "e1", event_type: "runtime.emergency_blackout.applied", category: "safety"}],
  );
  const encoded = JSON.stringify(projection);
  assert.match(encoded, /EXTERNAL_EXECUTION_UNKNOWN/);
  assert.match(encoded, /runtime\.emergency_blackout\.applied/);
  assert.doesNotMatch(encoded, /untrusted private endpoint response/);
});

test("Session comparison computes bounded metrics from recorded traces and reports differences", () => {
  const h = harness();
  const current = {session_id: "s1", name: "Current", runtime_snapshot_id: "snap-a", started_at: "2026-01-01T00:00:00Z", ended_at: "2026-01-01T00:00:10Z"};
  const compared = {session_id: "s2", name: "Previous", runtime_snapshot_id: "snap-b", started_at: "2026-01-01T00:00:00Z", ended_at: "2026-01-01T00:00:08Z"};
  const currentCues = [{result: "COMPLETED", actions: [{result: "COMPLETED"}, {result: "FAILED"}]}];
  const previousCues = [{result: "FAILED", interrupted: true, actions: [{result: "FAILED"}]}];
  const currentEvents = [{category: "safety"}, {category: "device"}];
  const previousEvents = [{category: "device"}];
  const metrics = h.sessionComparisonMetrics(current, currentCues, currentEvents);
  assert.equal(metrics.duration_ms, 10000);
  assert.equal(metrics.action_count, 2);
  assert.equal(metrics.failed_actions, 1);
  assert.equal(metrics.safety_events, 1);
  const markup = h.sessionComparisonMarkup(current, currentCues, currentEvents, compared, previousCues, previousEvents);
  assert.match(markup, /Current · Previous/);
  assert.match(markup, /Snapshot differs/);
  assert.match(markup, /\+2\.0 s/);
  assert.match(markup, /Interrupted Cues/);
});
