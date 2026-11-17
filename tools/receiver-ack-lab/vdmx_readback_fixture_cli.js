#!/usr/bin/env node
"use strict";

// Offline OSCQuery fixture inspector. STDIN -> parse -> one sanitized
// published-control summary. NEVER connects to VDMX, Hub or show devices.
const { readVDMXPublishedControl } = require("./vdmx_oscquery_readback");

const MAX_BYTES = 65536;
async function main() {
  const targetPath = process.argv[2];
  if (process.argv.length !== 3 || !targetPath) {
    throw new Error("usage: node vdmx_readback_fixture_cli.js /Published/Property < oscquery.json");
  }
  const chunks = [];
  let count = 0;
  for await (const chunk of process.stdin) {
    count += chunk.length;
    if (count > MAX_BYTES) throw new Error("OSCQuery fixture exceeded 64KiB");
    chunks.push(chunk);
  }
  const text = Buffer.concat(chunks).toString("utf8");
  if (!text) throw new Error("empty OSCQuery fixture");
  const json = JSON.parse(text);
  const property = readVDMXPublishedControl(json, targetPath);
  const summary = {
    path: property.property,
    value: property.value,
    evidence: property.evidence,
    // A local JSON file is NOT a qualified authenticated source.
    receiver_authenticated: false,
    snapshot_current: false,
    application_cue_acknowledged: false,
    video_output_verified: false,
    commands_enabled: false,
  };
  process.stdout.write(JSON.stringify(summary,null,2) + "\n");
}
if (require.main === module) {
  main().catch(() => {
    // No stack dump, private paths or raw fixture data in logs.
    process.stderr.write("Invalid/offline OSCQuery fixture; output UNKNOWN\n");
    process.exitCode = 1;
  });
}
module.exports = { main };
