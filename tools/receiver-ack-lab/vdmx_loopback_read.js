#!/usr/bin/env node
"use strict";

// EXPLICITLY MANUAL, LOOPBACK-ONLY OSCQuery HTTP GET for an isolated
// NON-SHOW VDMX 6 test project. NEVER uses a remote host, redirects,
// WebSockets, UDP, OSC send, Hub channels or Cue/GO/STOP commands.
//
// A localhost HTTP reply proves neither VDMX's authenticated identity
// nor that a Cue caused the reported control value. Never mark physical
// video verified or app receipt confirmed based on this diagnostic.
const http = require("node:http");
const { readVDMXPublishedControl } = require("./vdmx_oscquery_readback");

const MAX_BODY_BYTES = 65536;
const TIMEOUT_MS = 1800;
const TEST_CONFIRMATION = "STAGECORE_VDMX_DISPOSABLE_TEST_PROJECT";

function parseArgs(argv, env = process.env) {
  if (!Array.isArray(argv) || argv.length !== 4 ||
      argv[0] !== "--port" || argv[2] !== "--path" ||
      env.STAGECORE_VDMX_DISPOSABLE_TEST_PROJECT !== "1") {
    throw new Error("explicit disposable VDMX lab confirmation and --port/--path required");
  }
  const rawPort = argv[1], target = argv[3];
  if (!/^[1-9][0-9]{0,4}$/.test(rawPort)) {
    throw new Error("invalid explicit OSCQuery port");
  }
  const port = Number(rawPort);
  if (port < 1024 || port > 65535 ||
      typeof target !== "string" ||
      !/^\/[A-Za-z0-9_/-]{1,120}$/.test(target) ||
      target.includes("//") || target.includes("/../") ||
      target.endsWith("/") || target.includes("?") ||
      target.includes("#")) {
    throw new Error("invalid OSCQuery port or exact published path");
  }
  return { port, target };
}

// No URL argument and no configurable hostname = no remote requests,
// redirects to the LAN, credential URLs or hostname/DNS rebinding.
function getLoopbackControl({ port, target }, transport = http) {
  return new Promise((resolve, reject) => {
    let settled = false;
    function fail() {
      if (settled) return;
      settled = true;
      reject(new Error("loopback OSCQuery read failed; state UNKNOWN"));
    }
    const request = transport.request({
      host: "127.0.0.1",
      family: 4,
      port,
      path: target,
      method: "GET",
      agent: false,
      headers: { Accept: "application/json", Connection: "close" },
      signal: AbortSignal.timeout(TIMEOUT_MS),
    }, (response) => {
      // Never follow HTTP 30x Location, fetch arbitrary URL, or use
      // an OSCQuery reply as an authority to open a second socket.
      if (response.statusCode !== 200 ||
          !/^application\/json(?:\s*;|$)/i.test(String(response.headers["content-type"] || "")) ||
          response.headers["content-encoding"]) {
        response.destroy();
        fail();
        return;
      }
      let length = 0;
      const parts = [];
      response.on("data", (part) => {
        length += part.length;
        if (length > MAX_BODY_BYTES) {
          response.destroy();
          fail();
          return;
        }
        parts.push(part);
      });
      response.on("error", fail);
      response.on("end", () => {
        if (settled) return;
        settled = true;
        try {
          const json = JSON.parse(Buffer.concat(parts).toString("utf8"));
          const control = readVDMXPublishedControl(json, target);
          resolve({
            schema_version: 1,
            source: "UNAUTHENTICATED_LOOPBACK_OSCQUERY_TEST_GET",
            path: control.property,
            published_value: control.value,
            evidence: "PUBLISHED_CONTROL_VALUE_ONLY",
            captured_at_utc: new Date().toISOString(),
            // A one-off GET has NO heartbeat/subscription/receiver trust.
            receiver_authenticated: false,
            monitoring_active: false,
            historical_cue_execution_confirmed: false,
            physical_video_output_verified: false,
            commands_enabled: false,
            automatic_replay: false,
          });
        } catch (_) {
          reject(new Error("OSCQuery returned invalid control; state UNKNOWN"));
        }
      });
    });
    request.on("error", fail);
    request.end();
  });
}

async function main(argv = process.argv.slice(2), env = process.env) {
  const args = parseArgs(argv, env);
  const result = await getLoopbackControl(args);
  process.stdout.write(JSON.stringify(result, null, 2) + "\n");
}

if (require.main === module) {
  main().catch(() => {
    // No URLs, raw bodies, local process data or potentially sensitive
    // OSCQuery metadata in failure logs.
    process.stderr.write("VDMX lab read unavailable or invalid; outputs UNKNOWN\n");
    process.exitCode = 1;
  });
}

module.exports = { parseArgs, getLoopbackControl, main, TEST_CONFIRMATION };
