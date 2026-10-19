"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const http = require("node:http");
const { parseArgs, getLoopbackControl } = require("./vdmx_loopback_read");

const target = "/StageCore_Lab/layer_opacity";
const confirmed = { STAGECORE_VDMX_DISPOSABLE_TEST_PROJECT: "1" };
function leaf(value = 0.5) {
  // Exact-path OSCQuery GET may return a leaf, not a root CONTENTS tree.
  return { FULL_PATH: target, TYPE: "f", ACCESS: 1, VALUE: [value] };
}
async function mockServer(t, handler) {
  const server = http.createServer(handler);
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  t.after(() => new Promise((resolve) => server.close(resolve)));
  return server.address().port;
}

test("CLI requires explicit disposable project confirmation and exact target", () => {
  assert.throws(() => parseArgs(["--port", "9123", "--path", target], {}), /confirmation/);
  assert.deepEqual(parseArgs(["--port", "9123", "--path", target], confirmed), {
    port: 9123, target,
  });
  for (const args of [
    ["--port", "0", "--path", target],
    ["--port", "80", "--path", target],
    ["--port", "65536", "--path", target],
    ["--port", "0999", "--path", target],
    ["--port", "localhost:9000", "--path", target],
    ["--port", "9000", "--path", "http://127.0.0.1/admin"],
    ["--port", "9000", "--path", "/secret?HOST_INFO"],
    ["--port", "9000", "--path", "/bad//path"],
    ["--port", "9000", "--path", "/bad/../admin"],
    ["--port", "9000", "--path", "/bad#fragment"],
  ]) {
    assert.throws(() => parseArgs(args, confirmed), /invalid/);
  }
});

test("one local GET returns unauthenticated Control Surface value, never execution proof", async (t) => {
  let count = 0, requestPath = "", method = "", host = "";
  const port = await mockServer(t, (req, res) => {
    count++;
    requestPath = req.url;
    method = req.method;
    host = req.headers.host;
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify(leaf(0.75)));
  });
  const result = await getLoopbackControl({ port, target });
  assert.equal(count, 1);
  assert.equal(requestPath, target);
  assert.equal(method, "GET");
  assert.equal(host, "127.0.0.1:" + port);
  assert.equal(result.published_value, 0.75);
  assert.equal(result.path, target);
  assert.equal(result.source, "UNAUTHENTICATED_LOOPBACK_OSCQUERY_TEST_GET");
  assert.equal(result.evidence, "PUBLISHED_CONTROL_VALUE_ONLY");
  assert.match(result.captured_at_utc, /^\d{4}-\d\d-\d\dT/);
  for (const key of [
    "receiver_authenticated", "monitoring_active",
    "historical_cue_execution_confirmed", "physical_video_output_verified",
    "commands_enabled", "automatic_replay",
  ]) assert.equal(result[key], false, key + " must always remain false");
});

test("does not follow HTTP redirect, even to localhost", async (t) => {
  let otherServerHits = 0;
  const redirectedPort = await mockServer(t, (_, res) => {
    otherServerHits++;
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify(leaf()));
  });
  const redirectingPort = await mockServer(t, (_, res) => {
    res.writeHead(302, {
      Location: "http://127.0.0.1:" + redirectedPort + target,
    });
    res.end();
  });
  await assert.rejects(
    getLoopbackControl({ port: redirectingPort, target }), /UNKNOWN/
  );
  assert.equal(otherServerHits, 0);
});

test("fails closed on wrong path, missing JSON content type and malformed JSON", async (t) => {
  const wrongPath = await mockServer(t, (_, res) => {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ FULL_PATH: "/a-different-control",
      TYPE: "f", ACCESS: 1, VALUE: [1] }));
  });
  await assert.rejects(getLoopbackControl({ port: wrongPath, target }), /UNKNOWN/);

  const textType = await mockServer(t, (_, res) => {
    res.writeHead(200, { "Content-Type": "text/html" });
    res.end(JSON.stringify(leaf()));
  });
  await assert.rejects(getLoopbackControl({ port: textType, target }), /UNKNOWN/);

  const badJSON = await mockServer(t, (_, res) => {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end("not-json");
  });
  await assert.rejects(getLoopbackControl({ port: badJSON, target }), /UNKNOWN/);
});

test("oversized, compressed, and invalid numeric VALUE fail closed", async (t) => {
  const oversized = await mockServer(t, (_, res) => {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ ...leaf(), extra: "X".repeat(65536) }));
  });
  await assert.rejects(getLoopbackControl({ port: oversized, target }), /UNKNOWN/);

  const compressed = await mockServer(t, (_, res) => {
    res.writeHead(200, {
      "Content-Type": "application/json",
      "Content-Encoding": "gzip",
    });
    res.end(JSON.stringify(leaf()));
  });
  await assert.rejects(getLoopbackControl({ port: compressed, target }), /UNKNOWN/);

  const wrongType = await mockServer(t, (_, res) => {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ ...leaf(), VALUE: ["0.7"] }));
  });
  await assert.rejects(getLoopbackControl({ port: wrongType, target }), /UNKNOWN/);
});

test("no local OSCQuery service means UNKNOWN, without UDP or fallback", async () => {
  const server = http.createServer();
  await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
  const port = server.address().port;
  await new Promise(resolve => server.close(resolve));
  await assert.rejects(getLoopbackControl({ port, target }), /UNKNOWN/);
});

test("CLI source cannot accept remote addresses, credentials, or output commands", () => {
  const fs = require("node:fs");
  const path = require("node:path");
  const source = fs.readFileSync(path.join(__dirname, "vdmx_loopback_read.js"), "utf8");
  assert.match(source, /host: "127\.0\.0\.1"/);
  assert.match(source, /method: "GET"/);
  assert.match(source, /AbortSignal\.timeout/);
  assert.match(source, /STAGECORE_VDMX_DISPOSABLE_TEST_PROJECT/);
  assert.equal(source.includes("https.request("), false);
  assert.equal(source.includes("dgram.createSocket("), false);
  assert.equal(source.includes("WebSocket("), false);
  assert.equal(source.includes("execFile("), false);
  assert.equal(source.includes("method: \"POST\""), false);
});
