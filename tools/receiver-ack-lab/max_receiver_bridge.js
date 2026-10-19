"use strict";

// Experimental Node for Max adapter only. NO UDP sockets, MIDI calls,
// file writes, or Cue dispatch. Keep disabled unless explicitly enabled
// in a non-show lab. This does not prove who supplied a Max message.
const { AppObserverLab, LIVE_SLOT, VDMX_PROPERTY } = require("./app_observer");
const crypto = require("node:crypto");

function parsePrimitive(raw) {
  const v = JSON.parse(String(raw));
  if ((typeof v !== "string" && typeof v !== "number" && typeof v !== "boolean") ||
      (typeof v === "number" && !Number.isFinite(v))) {
    throw new Error("property observation must be one primitive");
  }
  return v;
}

function registerMaxBridge(maxApi, tracker, now = Date.now) {
  if (!maxApi || typeof maxApi.addHandler !== "function" ||
      typeof maxApi.outlet !== "function" ||
      !(tracker instanceof AppObserverLab)) {
    throw new Error("Max API and observer tracker required");
  }
  const liveLast = new Map();
  const vdmxLast = new Map();
  let sequence = 0;
  let observerEpoch = 0;
  async function maybeObserve(input) {
    sequence += 1;
    const packets = tracker.observe({
      ...input, observation_sequence: sequence,
      observed_at_ms: now(), change: true,
    });
    // Max callbacks are async. A cancel/Track rebinding may arrive while
    // the first outlet is still pending. Never emit a second stale packet
    // after the observer epoch changes; no callback sends network traffic.
    const epoch = observerEpoch;
    for (const packet of packets) {
      if (epoch !== observerEpoch) break;
      await maxApi.outlet("stagecore_ack_lab_base64", packet.toString("base64"));
    }
  }
  function safeHandler(action) {
    return async (...args) => {
      try {
        await action(...args);
      } catch (_) {
        // Fail closed. Never log private keys or raw receipt metadata.
        if (typeof maxApi.post === "function") {
          await maxApi.post("StageCore ACK lab: rejected observation/metadata");
        }
      }
    };
  }
  // LAB_ONLY fixture. This does not use real Project, Companion, Cue,
  // Execution or Snapshot IDs, cannot authenticate with a Hub and is
  // not usable for runtime GO. Requires a current local Track 0 baseline.
  maxApi.addHandler("stagecore_arm_live_fixture", safeHandler(async (slotRaw) => {
    const slot = Number(slotRaw);
    if (!Number.isSafeInteger(slot) || slot < 0 || slot > 256 ||
        !liveLast.has(0)) throw new Error("lab fixture needs Live Track 0 baseline");
    const nowMs = now();
    const labID = crypto.randomUUID();
    tracker.arm({
      receipt: {
        project_id: "LAB_ONLY", runtime_snapshot_id: "LAB_ONLY",
        machine_role_id: "LAB_TRACK_0", companion_id: "LAB_ONLY",
        receiver_instance_id: "LAB_MAX_SCRIPT", kind: "ABLETON",
        connection_generation: "1",
        execution_id: labID, cue_id: "LAB_ONLY",
        challenge: crypto.randomBytes(20).toString("hex"),
      },
      expect: { source: LIVE_SLOT, track_index: 0,
        playing_slot_index: slot },
      expires_at_ms: nowMs + 10000,
    });
    observerEpoch += 1;
    if (typeof maxApi.post === "function") {
      await maxApi.post("StageCore LAB_ONLY: manually arm Live Track 0; no commands sent");
    }
  }));
  maxApi.addHandler("stagecore_arm_json", safeHandler(async (...parts) => {
    // Node for Max may pass a serialized JSON Max symbol as one atom, or
    // split whitespace-delimited atoms. Treat both as one lab document.
    // Never accept credentials through this handler.
    const raw = parts.map((part) => String(part)).join(" ");
    if (Buffer.byteLength(raw, "utf8") > 4096) throw new Error("lab arm too large");
    const data = JSON.parse(raw);
    tracker.arm(data); // no signed ACK, no command
    observerEpoch += 1;
  }));
  maxApi.addHandler("stagecore_reset_observer", safeHandler(async () => {
    // Called on Live API object-ID rebinding (different Live Set/Track).
    // No stale baseline or in-flight command can survive reattachment.
    observerEpoch += 1;
    liveLast.clear();
    vdmxLast.clear();
    tracker.cancel();
  }));
  maxApi.addHandler("stagecore_live_slot", safeHandler(async (track, slot) => {
    const trackIndex = Number(track), playingSlotIndex = Number(slot);
    if (!Number.isSafeInteger(trackIndex) || trackIndex < 0 ||
        trackIndex > 1024 || !Number.isSafeInteger(playingSlotIndex)) return;
    if (!liveLast.has(trackIndex) && liveLast.size >= 1024) {
      tracker.cancel();
      return;
    }
    // live.observer sends its current value upon subscription/bang. That
    // first baseline MUST NEVER generate a RECEIVED or APPLIED receipt.
    const previous = liveLast.get(trackIndex);
    liveLast.set(trackIndex, playingSlotIndex);
    if (previous === undefined || previous === playingSlotIndex) return;
    await maybeObserve({
      source: LIVE_SLOT, track_index: trackIndex,
      playing_slot_index: playingSlotIndex,
    });
  }));
  maxApi.addHandler("stagecore_vdmx_property", safeHandler(async (property, rawValue) => {
    if (typeof property !== "string" ||
        !/^\/[a-zA-Z0-9_/-]{1,120}$/.test(property)) return;
    // Input MUST come from an application-side published property
    // observer, NOT from the UDP packet receive path.
    const value = parsePrimitive(rawValue);
    if (!vdmxLast.has(property) && vdmxLast.size >= 512) {
      tracker.cancel();
      return;
    }
    const prev = vdmxLast.get(property);
    vdmxLast.set(property, value);
    if (prev === undefined || prev === value) return;
    await maybeObserve({ source: VDMX_PROPERTY, property, value });
  }));
  maxApi.addHandler("stagecore_cancel", safeHandler(async () => {
    observerEpoch += 1;
    tracker.cancel();
  }));
}

function startLabAdapter() {
  const explicitlyStarted = process.argv.includes("--lab") ||
    process.env.STAGECORE_ACK_LAB_ENABLED === "1";
  if (!explicitlyStarted ||
      !["max", "maxforlive"].includes(process.env.MAX_ENV)) {
    throw new Error("StageCore receiver lab requires manual script start --lab inside Max");
  }
  // By default, this test-only process gets a NEW non-exported key.
  // Its signature is deliberately unusable by any Hub-side verifier.
  const hex = process.env.STAGECORE_LAB_RECEIVER_KEY_HEX || "";
  if (hex && !/^(?:[0-9a-fA-F]{2}){32,}$/.test(hex)) {
    throw new Error("StageCore receiver lab key must have >=32 bytes");
  }
  const key = hex ? Buffer.from(hex, "hex") : crypto.randomBytes(32);
  const maxApi = require("max-api");
  const tracker = new AppObserverLab({ key });
  registerMaxBridge(maxApi, tracker);
}
if (require.main === module) startLabAdapter();
module.exports = { registerMaxBridge, parsePrimitive };
