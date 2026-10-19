"use strict";

// OFFLINE OSCQuery readback freshness/identity model only.
// The calling transport must independently authenticate the actual
// VDMX receiver and authorize Project/Role/Snapshot; OSCQuery JSON
// itself and peer IP/port DO NOT authenticate the sender.
// No WebSocket/HTTP I/O, command dispatch, auto-replay or Cue ACK.
const { readVDMXPublishedControl } = require("./vdmx_oscquery_readback");
const { validPublishedPath } = require("./vdmx_oscquery_path");

const MAX_AGE_MS = 5000;
const MAX_NOTIFICATION_BYTES = 4096;
const UNCERTAIN = "UNKNOWN";
const VALID_SCOPE = /^[A-Za-z0-9_.:-]{1,128}$/;

function qualifiedScope(scope) {
  return scope && typeof scope === "object" && !Array.isArray(scope) &&
    ["project_id", "runtime_snapshot_id", "machine_role_id",
      "receiver_instance_id"].every((key) =>
      typeof scope[key] === "string" && VALID_SCOPE.test(scope[key])) &&
    Number.isSafeInteger(scope.generation) && scope.generation > 0;
}
function sameScope(a, b) {
  return !!a && !!b && a.project_id === b.project_id &&
    a.runtime_snapshot_id === b.runtime_snapshot_id &&
    a.machine_role_id === b.machine_role_id &&
    a.receiver_instance_id === b.receiver_instance_id &&
    a.generation === b.generation;
}
function pathTouchesTarget(path, target) {
  return path === "/" || path === target ||
    target.startsWith(path + "/") || path.startsWith(target + "/");
}
class VDMXOfflineReadbackSession {
  constructor({ targetPath, now = Date.now, maxAgeMs = MAX_AGE_MS }) {
    if (!validPublishedPath(targetPath) || typeof now !== "function" ||
        !Number.isSafeInteger(maxAgeMs) || maxAgeMs <= 0 ||
        maxAgeMs > 30000) {
      throw new Error("invalid offline VDMX readback settings");
    }
    this.targetPath = targetPath;
    this.now = now;
    this.maxAgeMs = maxAgeMs;
    this.scope = null;
    this.bound = false;
    this.lastClock = now();
    this.lastSequence = 0;
    this.lastObservedAt = null;
    this.value = null;
    this.condition = UNCERTAIN;
    this.reason = "NO_VERIFIED_OBSERVATION";
  }
  invalidate(reason) {
    this.value = null;
    this.lastObservedAt = null;
    this.condition = UNCERTAIN;
    this.reason = reason;
  }
  checkClock() {
    const clock = this.now();
    if (!Number.isSafeInteger(clock) || clock < this.lastClock) {
      this.invalidate("CLOCK_UNCERTAIN");
      this.bound = false; // requires a strictly NEWER generation
      throw new Error("clock uncertain; new qualified generation required");
    }
    this.lastClock = clock;
    return clock;
  }
  // Explicitly called ONLY after the external, authenticated local adapter
  // has verified receiver identity and the requested Project/Role/Snapshot.
  // This method CANNOT perform that authentication on its own.
  bindQualified(scope) {
    this.checkClock();
    if (!qualifiedScope(scope) ||
        (this.scope && (
          scope.project_id !== this.scope.project_id ||
          scope.runtime_snapshot_id !== this.scope.runtime_snapshot_id ||
          scope.machine_role_id !== this.scope.machine_role_id ||
          scope.receiver_instance_id !== this.scope.receiver_instance_id ||
          scope.generation <= this.scope.generation))) {
      throw new Error("untrusted or old OSCQuery readback scope");
    }
    this.scope = { ...scope };
    this.bound = true;
    this.lastSequence = 0;
    this.invalidate("AWAITING_FRESH_SNAPSHOT");
  }
  ensureScope(scope, sequence, observedAtMs) {
    const now = this.checkClock();
    if (!this.bound || !sameScope(scope, this.scope) ||
        !Number.isSafeInteger(sequence) || sequence <= this.lastSequence ||
        !Number.isSafeInteger(observedAtMs) || observedAtMs > now ||
        now - observedAtMs > this.maxAgeMs ||
        (this.lastObservedAt !== null && observedAtMs < this.lastObservedAt)) {
      throw new Error("stale, out-of-scope or replayed OSCQuery observation");
    }
    return now;
  }
  // A fresh network snapshot is never itself an application transition,
  // an OSC command ACK, or a physical video-output measurement.
  observeSnapshot(scope, sequence, observedAtMs, snapshot) {
    this.ensureScope(scope, sequence, observedAtMs);
    let view;
    try {
      view = readVDMXPublishedControl(snapshot, this.targetPath);
    } catch (_) {
      this.invalidate("INVALID_OR_UNPUBLISHED_CONTROL");
      throw new Error("invalid or unpublished OSCQuery control");
    }
    this.lastSequence = sequence;
    this.lastObservedAt = observedAtMs;
    this.value = view.value;
    this.condition = "CURRENT_PUBLISHED_CONTROL_ONLY";
    this.reason = "UNVERIFIED_APP_CONTROL_STATE";
    return this.view();
  }
  // Handles OSCQuery JSON address-space notifications from an already
  // authenticated adapter. A notification cannot set output VALUE.
  // PATH_CHANGED, PATH_RENAMED, PATH_REMOVED and PATH_ADDED invalidate
  // the target if their path touches the selected control.
  observeNotification(scope, sequence, observedAtMs, message) {
    this.ensureScope(scope, sequence, observedAtMs);
    let raw;
    try { raw = JSON.stringify(message); } catch (_) {
      this.invalidate("MALFORMED_OSCQUERY_NOTIFICATION");
      throw new Error("invalid OSCQuery notification");
    }
    if (!raw || Buffer.byteLength(raw, "utf8") > MAX_NOTIFICATION_BYTES ||
        !message || typeof message !== "object" || Array.isArray(message) ||
        !["PATH_CHANGED", "PATH_RENAMED", "PATH_REMOVED", "PATH_ADDED"].includes(message.COMMAND)) {
      this.invalidate("MALFORMED_OSCQUERY_NOTIFICATION");
      throw new Error("invalid OSCQuery notification");
    }
    let paths;
    if (message.COMMAND === "PATH_RENAMED") {
      if (!message.DATA || typeof message.DATA !== "object" ||
          Array.isArray(message.DATA)) {
        this.invalidate("MALFORMED_OSCQUERY_NOTIFICATION");
        throw new Error("invalid renamed OSCQuery path");
      }
      paths = [message.DATA.OLD, message.DATA.NEW];
    } else {
      paths = [message.DATA];
    }
    if (paths.some((p) => !validPublishedPath(p, true))) {
      this.invalidate("MALFORMED_OSCQUERY_NOTIFICATION");
      throw new Error("invalid changed OSCQuery path");
    }
    this.lastSequence = sequence;
    if (paths.some((p) => pathTouchesTarget(p, this.targetPath))) {
      this.invalidate("OSCQUERY_PATH_CHANGED");
    }
    return this.view();
  }
  disconnect(scope) {
    if (!this.bound || !sameScope(scope, this.scope)) return false;
    this.bound = false;
    this.invalidate("CONNECTION_LOST");
    return true;
  }
  view() {
    const clock = this.checkClock();
    if (this.lastObservedAt !== null &&
        clock - this.lastObservedAt > this.maxAgeMs) {
      this.invalidate("SNAPSHOT_EXPIRED");
    }
    return {
      target_path: this.targetPath,
      condition: this.condition,
      reason: this.reason,
      published_value: this.condition === "CURRENT_PUBLISHED_CONTROL_ONLY" ?
        this.value : null,
      software_control_report_current: this.condition === "CURRENT_PUBLISHED_CONTROL_ONLY",
      authenticated_receiver: false, // only external adapter could qualify
      application_receipt_confirmed: false,
      physical_video_output_verified: false,
      commands_enabled: false,
      automated_replay: false,
    };
  }
}
module.exports = { VDMXOfflineReadbackSession };
