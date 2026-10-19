"use strict";

// OSCQuery JSON snapshot parser ONLY. No fetch, TCP/UDP/WebSocket, OSC send,
// publication, or GO/STOP. An offline fixture is never proof of an actual
// VDMX pixel, screen, video or of an authenticated VDMX application.
const MAX_JSON_BYTES = 65536;
const MAX_DEPTH = 12;
const MAX_NODES = 256;
const { validPublishedPath, validChildName } = require("./vdmx_oscquery_path");

function own(object, key) {
  return Object.prototype.hasOwnProperty.call(object, key);
}
function singleValue(node) {
  // OSCQuery permits a scalar VALUE for one argument as well as the
  // one-element array representation. Never collapse a multi-value
  // parameter: this observer supports exactly one typed argument.
  const present = node && Object.prototype.hasOwnProperty.call(node, "VALUE");
  const allowed = node && (node.ACCESS === undefined
    ? present
    : Number.isInteger(node.ACCESS) && node.ACCESS >= 0 &&
      node.ACCESS <= 3 && (node.ACCESS & 1) === 1);
  if (!allowed || !present || typeof node.TYPE !== "string" ||
      node.TYPE.length !== 1) {
    throw new Error("OSCQuery node is missing readable single VALUE/TYPE");
  }
  if (Array.isArray(node.VALUE) && node.VALUE.length !== 1) {
    throw new Error("OSCQuery node is missing readable single VALUE/TYPE");
  }
  const value = Array.isArray(node.VALUE) ? node.VALUE[0] : node.VALUE;
  if (["f", "d"].includes(node.TYPE)) {
    if (typeof value !== "number" || !Number.isFinite(value)) {
      throw new Error("invalid finite floating point OSCQuery value");
    }
  } else if (["i", "h"].includes(node.TYPE)) {
    if (!Number.isSafeInteger(value)) throw new Error("invalid OSCQuery integer");
  } else if (node.TYPE === "s") {
    if (typeof value !== "string" || value.length > 256) {
      throw new Error("invalid OSCQuery string");
    }
  } else if (node.TYPE === "T") {
    if (value !== true) throw new Error("invalid OSCQuery TRUE");
  } else if (node.TYPE === "F") {
    if (value !== false) throw new Error("invalid OSCQuery FALSE");
  } else {
    throw new Error("unsupported OSCQuery type for lab");
  }
  return value;
}

// OSCQuery FULL_PATH + CONTENTS tree, as documented by VIDVOX's
// OSCQueryProposal. Reject mismatches between CONTENTS keys and FULL_PATH.
// Limit recursion and size to protect the future receiving-app adapter.
// This function accepts already-obtained JSON, NEVER accesses network.
function readVDMXPublishedControl(snapshot, targetPath) {
  if (!validPublishedPath(targetPath) || !snapshot || typeof snapshot !== "object" ||
      Array.isArray(snapshot)) throw new Error("invalid OSCQuery root/target");

  const bytes = Buffer.byteLength(JSON.stringify(snapshot), "utf8");
  if (bytes > MAX_JSON_BYTES) throw new Error("OSCQuery fixture too large");
  let scanned = 0;
  function walk(node, expected, depth) {
    scanned++;
    if (scanned > MAX_NODES || depth > MAX_DEPTH ||
        !node || typeof node !== "object" || Array.isArray(node) ||
        node.FULL_PATH !== expected) {
      throw new Error("OSCQuery FULL_PATH or resource bound mismatch");
    }
    if (expected === targetPath) {
      // Reading published Control Surface value is APP REPORTED STATE
      // only; it doesn't prove a commanded layer actually rendered.
      return { property: targetPath, value: singleValue(node),
        evidence: "PUBLISHED_CONTROL_VALUE_ONLY",
        actualVideoOutputVerified: false,
        commandsEnabled: false,
        authenticatedReceiver: false };
    }
    if (!own(node,"CONTENTS")) return null;
    if (!node.CONTENTS || typeof node.CONTENTS !== "object" ||
        Array.isArray(node.CONTENTS)) throw new Error("bad OSCQuery CONTENTS");
    const keys = Object.keys(node.CONTENTS);
    if (keys.length > MAX_NODES) throw new Error("OSCQuery too many children");
    for (const key of keys) {
      if (!validChildName(key)) {
        throw new Error("invalid OSCQuery CONTENTS child");
      }
      const childPath = expected === "/" ? "/" + key : expected + "/" + key;
      if (!validPublishedPath(childPath)) throw new Error("invalid OSCQuery child path");
      // Traverse only the selected target's ancestors; don't need to
      // trust unrelated published controls in an untrusted fixture.
      if (targetPath === childPath || targetPath.startsWith(childPath + "/")) {
        return walk(node.CONTENTS[key], childPath, depth + 1);
      }
    }
    return null;
  }
  if (snapshot.FULL_PATH !== "/" &&
      !validPublishedPath(snapshot.FULL_PATH)) throw new Error("invalid OSCQuery root path");
  if (snapshot.FULL_PATH !== "/" &&
      !(targetPath === snapshot.FULL_PATH ||
        targetPath.startsWith(snapshot.FULL_PATH + "/"))) {
    throw new Error("target not within published root");
  }
  const view = walk(snapshot, snapshot.FULL_PATH, 0);
  if (!view) throw new Error("unpublished OSCQuery property");
  return view;
}
module.exports = { readVDMXPublishedControl };
