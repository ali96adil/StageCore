"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const http = require("node:http");
const { compareManual } = require("./vdmx_manual_compare");
const { getLoopbackControl } = require("./vdmx_loopback_read");

const path = "/StageCore_Lab/layer_opacity";
const portForUnit = 9123;
const baseTime = Date.parse("2026-10-19T11:00:00.000Z");
function sample(value, overrides = {}) {
  return {
    schema_version: 1,
    source: "UNAUTHENTICATED_LOOPBACK_OSCQUERY_TEST_GET",
    path,
    published_value: value,
    evidence: "PUBLISHED_CONTROL_VALUE_ONLY",
    captured_at_utc: new Date(baseTime).toISOString(),
    receiver_authenticated: false,
    monitoring_active: false,
    historical_cue_execution_confirmed: false,
    physical_video_output_verified: false,
    commands_enabled: false,
    automatic_replay: false,
    ...overrides,
  };
}

test("second GET cannot occur before explicit manual confirmation", async () => {
  const values = [sample(0.25), sample(0.75)];
  let requests = 0;
  let release;
  let entered;
  const waiting = new Promise((resolve) => { release = resolve; });
  const atPrompt = new Promise((resolve) => { entered = resolve; });
  let clock = baseTime;
  const attempt = compareManual({ port: portForUnit, target: path }, {
    confirmed: true,
    now: () => clock,
    read: async () => values[requests++],
    confirm: async () => {
      entered();
      await waiting;
      clock += 100;
    },
  });
  await atPrompt;
  assert.equal(requests, 1, "read ran twice before human action");
  release();
  const result = await attempt;
  assert.equal(requests, 2);
  assert.equal(result.outcome, "PUBLISHED_CONTROL_VALUE_DIFFERED");
  assert.equal(result.software_value_differed, true);
  assert.equal(result.requests_made, 2);
  assert.equal(result.before_published_value, 0.25);
  assert.equal(result.after_published_value, 0.75);
  for (const key of [
    "receiver_authenticated", "application_cue_acknowledged",
    "change_caused_by_stagecore_verified", "physical_video_output_verified",
    "commands_enabled", "automatic_replay", "monitoring_active",
  ]) assert.equal(result[key], false, key);
});

test("no change is a report, not proof of any VDMX output being safe", async () => {
  const result = await compareManual({ port: portForUnit, target: path }, {
    confirmed: true,
    now: () => baseTime,
    read: async () => sample(0),
    confirm: async () => {},
  });
  assert.equal(result.outcome, "NO_VALUE_DIFFERENCE_OBSERVED");
  assert.equal(result.software_value_differed, false);
  assert.equal(result.physical_video_output_verified, false);
});

test("published VALUE type changes are inconclusive, not successful", async () => {
  let readCount = 0;
  const result = await compareManual({ port: portForUnit, target: path }, {
    confirmed: true,
    now: () => baseTime,
    read: async () => ++readCount === 1 ? sample(1) : sample("1"),
    confirm: async () => {},
  });
  assert.equal(result.outcome, "INCONCLUSIVE_TYPE_CHANGED");
  assert.equal(result.software_value_differed, null);
  assert.equal(result.application_cue_acknowledged, false);
});

test("wrong source, Project-like metadata, stale flags and nonfinite values fail closed", async () => {
  const changes = [
    { source: "TRUSTED_VDMX_RECEIVER" },
    { path: "/AnotherProject/layer_opacity" },
    { receiver_authenticated: true },
    { historical_cue_execution_confirmed: true },
    { physical_video_output_verified: true },
    { monitoring_active: true },
    { automatic_replay: true },
    { commands_enabled: true },
    { evidence: "APPLICATION_APPLIED" },
    { published_value: NaN },
    { captured_at_utc: "invalid" },
  ];
  for (const override of changes) {
    let reads = 0, confirms = 0;
    await assert.rejects(compareManual({ port: portForUnit, target: path }, {
      confirmed: true, now: () => baseTime,
      read: async () => { reads++; return sample(0.1, override); },
      confirm: async () => { confirms++; },
    }), /unqualified/);
    assert.equal(reads, 1);
    assert.equal(confirms, 0, "invalid first read prompted user to continue");
  }
});

test("manual cancellation and second GET failure never emit a stale result", async () => {
  let reads = 0;
  await assert.rejects(compareManual({ port: portForUnit, target: path }, {
    confirmed: true, now: () => baseTime,
    read: async () => { reads++; return sample(0.2); },
    confirm: async () => { throw new Error("operator cancelled"); },
  }), /cancelled/);
  assert.equal(reads, 1);
  reads = 0;
  await assert.rejects(compareManual({ port: portForUnit, target: path }, {
    confirmed: true, now: () => baseTime,
    read: async () => {
      reads++;
      if (reads === 2) throw new Error("VDMX disconnected");
      return sample(0.2);
    },
    confirm: async () => {},
  }), /disconnected/);
  assert.equal(reads, 2);
});

test("clock rollback and expired manual window are unknown", async () => {
  for (const nextTime of [baseTime - 1, baseTime + 120001]) {
    let step = 0, reads = 0;
    await assert.rejects(compareManual({ port: portForUnit, target: path }, {
      confirmed: true,
      now: () => step++ < 2 ? baseTime : nextTime,
      read: async () => { reads++; return sample(0.3); },
      confirm: async () => {},
    }), /uncertain/);
    assert.equal(reads, 1);
  }
});

test("missing disposable-project confirmation, unsafe port and path reject before reading", async () => {
  for (const options of [
    { port: 80, target: path },
    { port: "9123", target: path },
    { port: portForUnit, target: "http://127.0.0.1/private" },
    { port: portForUnit, target: "/secret?HOST_INFO" },
  ]) {
    let reads = 0;
    await assert.rejects(compareManual(options, {
      confirmed: true, now: () => baseTime,
      read: async () => { reads++; return sample(1); },
      confirm: async () => {},
    }), /invalid/);
    assert.equal(reads, 0);
  }
  await assert.rejects(compareManual({ port: portForUnit, target: path }, {
    confirmed: false,
    read: async () => sample(0.1), confirm: async () => {},
  }), /confirmation/);
});

test("two local GETs, no user action sent to VDMX by StageCore", async (t) => {
  let value = 0.2, requestCount = 0;
  const server = http.createServer((req, res) => {
    requestCount++;
    assert.equal(req.method, "GET");
    assert.equal(req.url, path);
    assert.equal(req.headers.accept, "application/json");
    res.writeHead(200, { "content-type": "application/json" });
    res.end(JSON.stringify({
      FULL_PATH: path, TYPE: "f", ACCESS: 1, VALUE: value,
    }));
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  t.after(() => new Promise((resolve) => server.close(resolve)));
  const result = await compareManual({
    port: server.address().port, target: path,
  }, {
    confirmed: true,
    read: getLoopbackControl,
    confirm: async () => {
      assert.equal(requestCount, 1, "second request was not manual");
      value = 0.8; // fake local test server; no VDMX command
    },
  });
  assert.equal(requestCount, 2);
  assert.equal(result.software_value_differed, true);
  assert.equal(result.application_cue_acknowledged, false);
});
