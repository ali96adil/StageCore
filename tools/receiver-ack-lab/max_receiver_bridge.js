"use strict";

// Experimental Node for Max adapter only. NO UDP sockets, MIDI calls,
// file writes, or Cue dispatch. Keep disabled unless explicitly enabled
// in a non-show lab. This does not prove who supplied a Max message.
const { AppObserverLab, LIVE_SLOT, VDMX_PROPERTY } = require("./app_observer");

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
  async function maybeObserve(input) {
    sequence += 1;
    const packets = tracker.observe({
      ...input, observation_sequence: sequence,
      observed_at_ms: now(), change: true,
    });
    // The Max outlet gives a lab fixture only. NO transport to Hub, no
    // hidden socket. Other Max objects MUST NOT forward this to a show.
    for (const packet of packets) {
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
  maxApi.addHandler("stagecore_arm_json", safeHandler(async (raw) => {
    const data = JSON.parse(String(raw));
    tracker.arm(data); // no signed ACK, no command
  }));
  maxApi.addHandler("stagecore_live_slot", safeHandler(async (track, slot) => {
    const trackIndex = Number(track), playingSlotIndex = Number(slot);
    if (!Number.isSafeInteger(trackIndex) || trackIndex < 0 ||
        !Number.isSafeInteger(playingSlotIndex)) return;
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
    const prev = vdmxLast.get(property);
    vdmxLast.set(property, value);
    if (prev === undefined || prev === value) return;
    await maybeObserve({ source: VDMX_PROPERTY, property, value });
  }));
  maxApi.addHandler("stagecore_cancel", safeHandler(async () => {
    tracker.cancel();
  }));
}

function startLabAdapter() {
  if (process.env.STAGECORE_ACK_LAB_ENABLED !== "1" ||
      !["max", "maxforlive"].includes(process.env.MAX_ENV)) {
    throw new Error("StageCore receiver lab requires explicit Max lab enablement");
  }
  const hex = process.env.STAGECORE_LAB_RECEIVER_KEY_HEX || "";
  if (!/^(?:[0-9a-fA-F]{2}){32,}$/.test(hex)) {
    throw new Error("StageCore receiver lab needs a private 32+ byte test key");
  }
  const maxApi = require("max-api");
  const tracker = new AppObserverLab({ key: Buffer.from(hex, "hex") });
  registerMaxBridge(maxApi, tracker);
}
if (require.main === module) startLabAdapter();
module.exports = { registerMaxBridge, parsePrimitive };
