"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const { AppObserverLab, LIVE_SLOT, VDMX_PROPERTY } = require("./app_observer");

function lab(kind = "ABLETON") {
  let ms = 1_800_000_000_000;
  const clock = () => ms;
  const key = Buffer.alloc(32, 0x61); // test-only, never a live receiver key
  const tracker = new AppObserverLab({ key, now: clock });
  const metadata = {
    project_id: "show-a", runtime_snapshot_id: "published-1",
    machine_role_id: kind === "ABLETON" ? "audio-ableton" : "video-vdmx",
    companion_id: "mac-a", receiver_instance_id: kind === "ABLETON" ? "max-live-1" : "vdmx-1",
    kind, connection_generation: "33", execution_id: "exec-1",
    cue_id: "cue-1", challenge: "test-only-unique-2026",
  };
  const expect = kind === "ABLETON" ?
    { source: LIVE_SLOT, track_index: 2, playing_slot_index: 5 } :
    { source: VDMX_PROPERTY, property: "/output/layerA/opacity", value: 1 };
  const command = () => ({ receipt: { ...metadata }, expect: { ...expect }, expires_at_ms: ms + 4000 });
  const event = (sequence, changes = {}) => ({
    ...expect, observation_sequence: sequence, observed_at_ms: ms,
    change: true, ...changes,
  });
  return { tracker, metadata, key, command, event, advance: n => { ms += n; }, now: () => ms };
}

test("Live track observation must be a new, matching change after arming", () => {
  const f = lab();
  assert.deepEqual(f.tracker.arm(f.command()), { armed: true, receiptPackets: [] });
  assert.deepEqual(f.tracker.observe(f.event(1, { change: false })), []);
  assert.deepEqual(f.tracker.observe(f.event(2, { track_index: 3 })), []);
  assert.deepEqual(f.tracker.observe(f.event(3, { playing_slot_index: 4 })), []);
  assert.deepEqual(f.tracker.observe(f.event(4, { observed_at_ms: f.now() - 10000 })), []);
  assert.deepEqual(f.tracker.observe(f.event(5, { observed_at_ms: f.now() + 1 })), []);
  assert.deepEqual(f.tracker.observe(f.event(6, { source: "MIDI_NOTE_ON" })), []);
  const signed = f.tracker.observe(f.event(7));
  assert.equal(signed.length, 2);
  assert(signed[0].includes(Buffer.from("APP_REPORTED_RECEIVED")));
  assert(signed[1].includes(Buffer.from("APP_REPORTED_APPLIED")));
  assert.equal(f.tracker.observe(f.event(8)).length, 0);
  assert.equal(f.tracker.status().physicalOutputVerified, false);
  assert.equal(f.tracker.status().commandsEnabled, false);
  assert.equal(f.tracker.status().networkEnabled, false);
  assert.equal(f.tracker.status().autoReplay, false);
  assert.throws(() => f.tracker.arm(f.command()), /already observed/i);
});

test("VDMX only emits on an observed published property matching the target", () => {
  const f = lab("VDMX");
  f.tracker.arm(f.command());
  assert.deepEqual(f.tracker.observe(f.event(1, { property: "/output/another/opacity" })), []);
  assert.deepEqual(f.tracker.observe(f.event(2, { value: 0 })), []);
  assert.deepEqual(f.tracker.observe(f.event(3, { source: "OSC_PACKET_RECEIVED" })), []);
  assert.equal(f.tracker.observe(f.event(4)).length, 2);
  assert.equal(f.tracker.observe(f.event(5)).length, 0);
});

test("a pending or expired command does not imply an application receipt", () => {
  const f = lab();
  f.tracker.arm(f.command());
  assert.throws(() => f.tracker.arm(f.command()), /invalid, concurrent/i);
  f.advance(4000);
  assert.deepEqual(f.tracker.observe(f.event(1)), []);
  assert.equal(f.tracker.status().pending, false);
  assert.throws(() => f.tracker.arm(f.command()), /already observed/i);
});

test("cancellation and clock rollback fail closed without command replay", () => {
  const f = lab();
  f.tracker.arm(f.command());
  f.tracker.cancel();
  assert.deepEqual(f.tracker.observe(f.event(1)), []);
  assert.throws(() => f.tracker.arm(f.command()), /already observed/i);
  f.advance(-1);
  assert.throws(() => f.tracker.status(), /clock uncertain/);
  assert.throws(() => f.tracker.observe(f.event(2)), /clock uncertain/);
});

test("input validation prevents unsupported or unbounded observations", () => {
  const f = lab();
  assert.throws(() => f.tracker.arm({ ...f.command(),
    expect: { source: LIVE_SLOT, track_index: 2, playing_slot_index: -1 } }), /must target/);
  assert.throws(() => f.tracker.arm({ ...f.command(),
    expires_at_ms: f.now() + 40000 }), /expiry/);
  assert.throws(() => f.tracker.arm({ ...f.command(),
    receipt: { ...f.metadata, kind: "UNKNOWN" } }), /invalid/);
  const v = lab("VDMX");
  assert.throws(() => v.tracker.arm({ ...v.command(),
    expect: { source: VDMX_PROPERTY, property: "invalid", value: 1 } }), /published output/);
});

test("all messages are emitted only after explicit matching observation; never sent over network", () => {
  const f = lab();
  f.tracker.arm(f.command());
  assert.equal(f.tracker.status().pending, true);
  const packets = f.tracker.observe(f.event(1));
  assert.equal(packets.length, 2);
  packets.forEach((packet) => assert(packet.length < 4096));
  const fresh = lab("ABLETON");
  // A new process has NO imported state; it must not resume or emit
  // anything without an explicit newly-qualified command arm.
  assert.deepEqual(fresh.tracker.observe(fresh.event(1)), []);
});

for (const [kind, target, extra] of [
  ["VDMX_MEDIA_PLAYBACK", "deck-A", {asset_id: "asset-A", state: "PLAYING", playback_instance_id: "play-02", baseline_playback_instance_id: "play-01"}],
  ["VDMX_RENDER_FRAME", "canvas-A", {frame_produced: true}],
]) {
  test(kind + " requires a fresh, correlated application observation", () => {
    const f = lab("VDMX");
    const receipt = {...f.metadata, correlation_id: "request-1"};
    const expect = {
      source: kind, target_id: target, application_generation: "vdmx-2",
      baseline_sequence: 10, ...extra,
    };
    assert.deepEqual(f.tracker.arm({...f.command(), receipt, expect}).receiptPackets, []);
    f.advance(1);
    const evidence = {
      ...expect, ...receipt, change: true, observed_at_ms: f.now(), observation_sequence: 11,
    };
    for (const [i, wrong] of [
      {source: "OSC_PACKET_RECEIVED"}, {observation_sequence: 10}, {observed_at_ms: f.now() - 1},
      {application_generation: "old-vdmx"}, {connection_generation: "32"},
      {target_id: "unrelated-target"}, {execution_id: "old-execution"}, {challenge: "other-challenge"},
      {correlation_id: "different-request"}, {project_id: "other-project"},
      kind === "VDMX_RENDER_FRAME" ? {frame_produced: false} : {asset_id: "wrong-asset"},
      kind === "VDMX_RENDER_FRAME" ? {source: "VDMX_OUTPUT_PROPERTY"} : {state: "QUEUED"},
      kind === "VDMX_RENDER_FRAME" ? {receiver_instance_id: "another-receiver"} : {playback_instance_id: "play-01"},
    ].entries()) {
      assert.deepEqual(f.tracker.observe({...evidence, observation_sequence: 11 + i, ...wrong}), []);
    }
    evidence.observation_sequence = 40;
    assert.equal(f.tracker.observe(evidence).length, 2);
    assert.deepEqual(f.tracker.observe({...evidence, observation_sequence: 41}), []);
  });
}
