"use strict";

// Lab-only OBSERVATION gate, NOT a network receiver. Do not connect a
// synthetic Max bang, raw MIDI message or raw VDMX OSC arrival here and
// pretend it proves playback or an externally observable output.
const { encodeReceipt, fieldsFromReceipt } = require("./receiver_ack");

const LIVE_SLOT = "LIVE_TRACK_PLAYING_SLOT";
const VDMX_PROPERTY = "VDMX_OUTPUT_PROPERTY";
const MAX_COMMANDS = 256;

class AppObserverLab {
  constructor({ key, now = Date.now }) {
    if (!Buffer.isBuffer(key) || key.length < 32 || typeof now !== "function") {
      throw new Error("per-receiver lab signing key and clock required");
    }
    this.key = key;
    this.now = now;
    this.pending = null;
    this.seen = new Set();
    this.sequence = 0n;
    this.lastTime = now();
  }
  checkClock() {
    const next = this.now();
    if (!Number.isFinite(next) || next < this.lastTime) {
      this.pending = null;  // clock changes invalidate all armed commands
      throw new Error("clock uncertain");
    }
    this.lastTime = next;
    return next;
  }
  arm(command) {
    const instant = this.checkClock();
    if (!command || typeof command !== "object" || Array.isArray(command) ||
        !command.receipt || !command.expect || this.pending ||
        this.seen.size >= MAX_COMMANDS) {
      throw new Error("invalid, concurrent or exhausted lab command");
    }
    const metadata = { ...command.receipt, sequence: "1", status: "APP_REPORTED_RECEIVED" };
    fieldsFromReceipt(metadata); // validates all signed fields; no key output
    const expectation = command.expect;
    if (metadata.kind === "ABLETON") {
      if (expectation.source !== LIVE_SLOT ||
          !Number.isSafeInteger(expectation.track_index) || expectation.track_index < 0 ||
          !Number.isSafeInteger(expectation.playing_slot_index) ||
          expectation.playing_slot_index < 0) {
        throw new Error("Live observation must target track and playing clip slot");
      }
    } else if (metadata.kind === "VDMX") {
      if (expectation.source !== VDMX_PROPERTY ||
          typeof expectation.property !== "string" ||
          !/^\/[a-zA-Z0-9_/-]{1,120}$/.test(expectation.property) ||
          !["string", "number", "boolean"].includes(typeof expectation.value) ||
          (typeof expectation.value === "number" && !Number.isFinite(expectation.value))) {
        throw new Error("VDMX must observe explicit published output property");
      }
    } else {
      throw new Error("unsupported receiver");
    }
    const identity = metadata.execution_id;
    if (this.seen.has(identity)) throw new Error("Execution already observed");
    const expiresAt = Number(command.expires_at_ms);
    if (!Number.isSafeInteger(expiresAt) || expiresAt <= instant ||
        expiresAt - instant > 15000) {
      throw new Error("short bounded observation expiry required");
    }
    this.seen.add(identity);
    this.pending = { metadata, expectation: { ...expectation }, expiresAt, armedAt: instant,
      lastObservationSequence: 0 };
    // ARM produces NO receipt: a sender message alone proves nothing.
    return { armed: true, receiptPackets: [] };
  }
  observe(event) {
    const instant = this.checkClock();
    const pending = this.pending;
    if (!pending) return [];
    if (instant >= pending.expiresAt) {
      this.pending = null;
      return []; // no late acknowledgment
    }
    if (!event || typeof event !== "object" || Array.isArray(event) ||
        !Number.isSafeInteger(event.observation_sequence) ||
        event.observation_sequence <= pending.lastObservationSequence ||
        event.change !== true || !Number.isSafeInteger(event.observed_at_ms) ||
        event.observed_at_ms < pending.armedAt ||
        event.observed_at_ms > instant || instant - event.observed_at_ms > 1000) {
      return [];
    }
    const expectation = pending.expectation;
    let matches = false;
    if (expectation.source === LIVE_SLOT) {
      matches = event.source === LIVE_SLOT &&
        event.track_index === expectation.track_index &&
        event.playing_slot_index === expectation.playing_slot_index;
    } else if (expectation.source === VDMX_PROPERTY) {
      matches = event.source === VDMX_PROPERTY &&
        event.property === expectation.property &&
        event.value === expectation.value;
    }
    // Out-of-order mismatches cannot be replayed later as fresh evidence.
    pending.lastObservationSequence = event.observation_sequence;
    if (!matches) return [];
    // Both receipt states are emitted only AFTER the app-side property
    // is observed. This is still NOT independent proof of sound/video.
    const packets = [];
    for (const status of ["APP_REPORTED_RECEIVED", "APP_REPORTED_APPLIED"]) {
      this.sequence += 1n;
      const receipt = {
        ...pending.metadata, status, sequence: this.sequence.toString(),
      };
      packets.push(encodeReceipt(receipt, this.key));
    }
    this.pending = null;
    return packets;
  }
  cancel() {
    this.pending = null;
  }
  status() {
    this.checkClock();
    return { pending: this.pending !== null, physicalOutputVerified: false,
      commandsEnabled: false, networkEnabled: false, autoReplay: false };
  }
}
module.exports = { AppObserverLab, LIVE_SLOT, VDMX_PROPERTY };
