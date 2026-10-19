#!/usr/bin/env node
"use strict";

// Two explicitly human-separated reads of ONE local OSCQuery control in
// a disposable VDMX test project. A value change is NOT a StageCore Cue
// execution receipt, video playback proof, or authentication of VDMX.
// No timer-based polling, OSC/MIDI output, Hub integration or writes.
const { parseArgs, getLoopbackControl } = require("./vdmx_loopback_read");

const MAX_MANUAL_WINDOW_MS = 120000;
const EXPECTED_SOURCE = "UNAUTHENTICATED_LOOPBACK_OSCQUERY_TEST_GET";
const EXPECTED_EVIDENCE = "PUBLISHED_CONTROL_VALUE_ONLY";

function validateRead(sample, target) {
  if (!sample || typeof sample !== "object" || Array.isArray(sample) ||
      sample.schema_version !== 1 || sample.path !== target ||
      sample.source !== EXPECTED_SOURCE ||
      sample.evidence !== EXPECTED_EVIDENCE ||
      !["number", "string", "boolean"].includes(typeof sample.published_value) ||
      (typeof sample.published_value === "number" &&
       !Number.isFinite(sample.published_value)) ||
      sample.receiver_authenticated !== false ||
      sample.monitoring_active !== false ||
      sample.historical_cue_execution_confirmed !== false ||
      sample.physical_video_output_verified !== false ||
      sample.commands_enabled !== false ||
      sample.automatic_replay !== false ||
      typeof sample.captured_at_utc !== "string" ||
      !Number.isFinite(Date.parse(sample.captured_at_utc))) {
    throw new Error("unqualified OSCQuery observation; comparison UNKNOWN");
  }
  return sample.published_value;
}

// This function cannot proceed without an explicit disposable-project
// flag AND a separate user-confirm step after the first GET. Failure to
// read either sample rejects the entire comparison; previous values
// are never turned into "unchanged" or "confirmed".
async function compareManual({ port, target }, {
  confirmed = false,
  read = getLoopbackControl,
  confirm,
  now = Date.now,
} = {}) {
  if (confirmed !== true || typeof confirm !== "function" ||
      typeof now !== "function" || typeof read !== "function") {
    throw new Error("manual disposable VDMX test confirmation required");
  }
  // Do NOT coerce an untrusted direct-call port with String(port).
  // The helper's port is an integer, just like getLoopbackControl.
  // CLI parseArgs handles converting a strictly validated decimal.
  if (!Number.isSafeInteger(port) || port < 1024 || port > 65535) {
    throw new Error("invalid direct-call OSCQuery comparison port");
  }
  // parseArgs is the sole shared path allowlist for CLI and helper.
  parseArgs(["--port", String(port), "--path", target], {
    STAGECORE_VDMX_DISPOSABLE_TEST_PROJECT: "1",
  });
  const start = now();
  if (!Number.isSafeInteger(start)) throw new Error("uncertain lab clock");
  const first = await read({ port, target });
  const original = validateRead(first, target);
  const afterFirst = now();
  if (!Number.isSafeInteger(afterFirst) || afterFirst < start ||
      afterFirst - start > MAX_MANUAL_WINDOW_MS) {
    throw new Error("stale/uncertain first observation; comparison UNKNOWN");
  }
  // Only the operator's actual manual confirmation advances the lab.
  await confirm();
  const afterAction = now();
  if (!Number.isSafeInteger(afterAction) || afterAction < afterFirst ||
      afterAction - start > MAX_MANUAL_WINDOW_MS) {
    throw new Error("stale/uncertain manual observation window; comparison UNKNOWN");
  }
  const second = await read({ port, target });
  const changed = validateRead(second, target);
  const finished = now();
  if (!Number.isSafeInteger(finished) || finished < afterAction ||
      finished - start > MAX_MANUAL_WINDOW_MS) {
    throw new Error("stale/uncertain second observation; comparison UNKNOWN");
  }
  const sameType = typeof original === typeof changed;
  const differed = sameType ? original !== changed : null;
  return {
    schema_version: 1,
    test_mode: "DISPOSABLE_VDMX_LOCAL_MANUAL_TWO_GET",
    target_path: target,
    before_published_value: original,
    after_published_value: changed,
    outcome: !sameType ? "INCONCLUSIVE_TYPE_CHANGED" :
      differed ? "PUBLISHED_CONTROL_VALUE_DIFFERED" :
        "NO_VALUE_DIFFERENCE_OBSERVED",
    software_value_differed: differed,
    requests_made: 2,
    receiver_authenticated: false,
    application_cue_acknowledged: false,
    change_caused_by_stagecore_verified: false,
    physical_video_output_verified: false,
    commands_enabled: false,
    automatic_replay: false,
    monitoring_active: false,
  };
}

async function main(argv = process.argv.slice(2), env = process.env) {
  // No automated or piped confirmation; lab use must be interactive.
  const { port, target } = parseArgs(argv, env);
  if (!process.stdin.isTTY || !process.stdout.isTTY) {
    throw new Error("manual interactive terminal required");
  }
  const readline = require("node:readline/promises");
  const terminal = readline.createInterface({
    input: process.stdin, output: process.stdout,
  });
  try {
    process.stderr.write("First local read. Do not operate show devices.\n");
    const result = await compareManual({ port, target }, {
      confirmed: env.STAGECORE_VDMX_DISPOSABLE_TEST_PROJECT === "1",
      confirm: async () => {
        await terminal.question(
          "Manually change ONLY the disposable VDMX test slider; press Enter for second GET: ",
          { signal: AbortSignal.timeout(MAX_MANUAL_WINDOW_MS) });
      },
    });
    process.stdout.write(JSON.stringify(result, null, 2) + "\n");
  } finally {
    terminal.close();
  }
}
if (require.main === module) {
  main().catch(() => {
    // Never leak URLs, raw OSCQuery metadata, or stale first sample.
    process.stderr.write(
      "VDMX comparison unavailable/uncertain; result UNKNOWN\n");
    process.exitCode = 1;
  });
}
module.exports = { compareManual, validateRead, main };
