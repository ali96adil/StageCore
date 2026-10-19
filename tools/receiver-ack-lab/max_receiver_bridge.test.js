"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const { AppObserverLab } = require("./app_observer");
const { registerMaxBridge, parsePrimitive } = require("./max_receiver_bridge");

function fixture(kind = "ABLETON") {
  const ms = 1_800_000_000_000;
  const max = { handlers: {}, output: [], logs: [],
    addHandler(name, fn) { this.handlers[name] = fn; },
    async outlet(...args) { this.output.push(args); },
    async post(message) { this.logs.push(message); },
  };
  const tracker = new AppObserverLab({ key: Buffer.alloc(32, 0x43), now: () => ms });
  registerMaxBridge(max, tracker, () => ms);
  const receipt = {
    project_id: "project-1", runtime_snapshot_id: "snapshot-1",
    machine_role_id: "machine-1", companion_id: "companion-1",
    receiver_instance_id: "receiver-1", kind,
    connection_generation: "7", execution_id: "ex-1",
    cue_id: "cue-1", challenge: "non-production-123456789",
  };
  const expect = kind === "ABLETON" ?
    { source: "LIVE_TRACK_PLAYING_SLOT", track_index: 0, playing_slot_index: 3 } :
    { source: "VDMX_OUTPUT_PROPERTY", property: "/output/foreground/opacity", value: 1 };
  return { max, tracker, receipt, expect,
    arm: async () => max.handlers.stagecore_arm_json(JSON.stringify({
      receipt, expect, expires_at_ms: ms + 3000,
    })),
  };
}

test("Node for Max Live API baseline and raw unobserved messages never sign", async () => {
  const { max, arm } = fixture();
  await max.handlers.stagecore_live_slot(0, -1); // observer subscribes: initial state
  await arm();
  assert.equal(max.output.length, 0);
  await max.handlers.stagecore_live_slot(1, 3); // other Track
  await max.handlers.stagecore_live_slot(0, -1); // unchanged
  await max.handlers.stagecore_live_slot(0, 2); // wrong playing clip
  assert.equal(max.output.length, 0);
  await max.handlers.stagecore_live_slot(0, 3); // app change to expected
  assert.equal(max.output.length, 2);
  assert.equal(max.output[0][0], "stagecore_ack_lab_base64");
  assert(Buffer.from(max.output[0][1], "base64").includes(Buffer.from("APP_REPORTED_RECEIVED")));
  assert(Buffer.from(max.output[1][1], "base64").includes(Buffer.from("APP_REPORTED_APPLIED")));
  await max.handlers.stagecore_live_slot(0, 3);
  assert.equal(max.output.length, 2);
});

test("first Live observer notification arriving after arm is baseline, never an ACK", async () => {
  const { max, arm } = fixture();
  await arm();
  await max.handlers.stagecore_live_slot(0, 3);
  assert.equal(max.output.length, 0, "initial state must not be mistaken for newly applied");
  await max.handlers.stagecore_live_slot(0, 2);
  assert.equal(max.output.length, 0);
  await max.handlers.stagecore_live_slot(0, 3);
  assert.equal(max.output.length, 2);
});

test("VDMX property must be observed to change, not OSC input or first report", async () => {
  const { max, arm } = fixture("VDMX");
  await max.handlers.stagecore_vdmx_property("/output/foreground/opacity", "0");
  await arm();
  assert.equal(max.output.length, 0);
  await max.handlers.stagecore_vdmx_property("/input/osc/address", "1");
  await max.handlers.stagecore_vdmx_property("/output/foreground/opacity", "0");
  assert.equal(max.output.length, 0);
  await max.handlers.stagecore_vdmx_property("/output/foreground/opacity", "1");
  assert.equal(max.output.length, 2);
  await max.handlers.stagecore_cancel();
});

test("malformed metadata and observation cannot manufacture receipts", async () => {
  const { max, arm } = fixture();
  await max.handlers.stagecore_arm_json("{unparseable");
  assert.equal(max.logs.length, 1);
  await max.handlers.stagecore_live_slot(0, -1);
  await arm();
  await max.handlers.stagecore_live_slot("other", "bad");
  await max.handlers.stagecore_live_slot(0, -1);
  await max.handlers.stagecore_cancel();
  await max.handlers.stagecore_live_slot(0, 3);
  assert.equal(max.output.length, 0);
  assert.throws(() => parsePrimitive("{}"), /primitive/);
  assert.throws(() => parsePrimitive("NaN"));
});

test("Node for Max bridge never opens sockets or loads a real receiver key by itself", () => {
  const fs = require("node:fs");
  const path = require("node:path");
  const source = fs.readFileSync(path.join(__dirname, "max_receiver_bridge.js"), "utf8");
  for (const forbidden of ["dgram.createSocket", "net.createServer", "OSCExecutor", "sendto(", "midiout", "stagecore/go"]) {
    assert.equal(source.includes(forbidden), false, "unexpected output dispatch in lab bridge");
  }
});


test("live API Track rebinding clears baseline and cancels an armed lab Execution", async () => {
  const f = fixture();
  await f.max.handlers.stagecore_live_slot(0, -1);
  await f.arm();
  await f.max.handlers.stagecore_reset_observer(); // live.path changed object ID
  await f.max.handlers.stagecore_live_slot(0, 3); // new initial value
  assert.equal(f.max.output.length, 0, "new Live object baseline must not sign");
  assert.equal(f.tracker.status().pending, false);
  const next = { ...f.receipt, execution_id: "ex-after-rebind",
    challenge: "fresh-rebind-2026-001" };
  await f.max.handlers.stagecore_arm_json(JSON.stringify({
    receipt: next, expect: f.expect, expires_at_ms: 1_800_000_003_000,
  }));
  await f.max.handlers.stagecore_live_slot(0, 2);
  assert.equal(f.max.output.length, 0);
  await f.max.handlers.stagecore_live_slot(0, 3);
  assert.equal(f.max.output.length, 2);
});

test("arm handler reassembles whitespace-split JSON Max symbols without dispatch", async () => {
  const f = fixture();
  const document = JSON.stringify({
    receipt: f.receipt, expect: f.expect, expires_at_ms: 1_800_000_003_000,
  }, null, 2);
  await f.max.handlers.stagecore_arm_json(...document.split(/\s+/));
  assert.equal(f.tracker.status().pending, true);
  assert.equal(f.max.output.length, 0);
  assert.equal(f.max.logs.length, 0);
});
