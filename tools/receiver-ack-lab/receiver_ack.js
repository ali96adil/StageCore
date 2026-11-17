#!/usr/bin/env node
"use strict";

// Lab-only signed OSC acknowledgment encoder for Node for Max or a VDMX
// helper. Does NOT open a socket, control Live/VDMX, play media, send GO,
// receive a Hub command or claim physical output safety.
// A receiver-side integration must itself verify the action occurred,
// provision a per-instance key securely, and authenticate the sender.
const crypto = require("node:crypto");
const fs = require("node:fs");

const address = "/stagecore/receiver/ack/v1";
const prefix = Buffer.from("stagecore/receiver-ack/v1\0", "utf8");
const statuses = new Set(["APP_REPORTED_RECEIVED", "APP_REPORTED_APPLIED"]);
const kinds = new Set(["ABLETON", "VDMX"]);
const fieldNames = [
  "project_id", "runtime_snapshot_id", "machine_role_id",
  "companion_id", "receiver_instance_id", "kind",
  "connection_generation", "execution_id", "cue_id", "challenge",
  "sequence", "status",
];

function normalizedNumber(value) {
  const str = String(value);
  if (!/^[1-9][0-9]*$/.test(str) || BigInt(str) > 9223372036854775807n) {
    throw new Error("invalid receiver generation/sequence");
  }
  return str;
}
function fieldsFromReceipt(receipt) {
  if (!receipt || typeof receipt !== "object" || Array.isArray(receipt)) {
    throw new Error("receipt must be an object");
  }
  const fields = ["1", ...fieldNames.map((name) => {
    if (!(name in receipt)) throw new Error("missing receiver receipt field");
    return name === "connection_generation" || name === "sequence"
      ? normalizedNumber(receipt[name]) : receipt[name];
  })];
  if (!kinds.has(fields[6]) || !statuses.has(fields[12]) ||
      Buffer.byteLength(fields[10] || "", "utf8") < 16) {
    throw new Error("invalid receiver kind/status/challenge");
  }
  for (const field of fields) {
    if (typeof field !== "string" || !field ||
        Buffer.byteLength(field, "utf8") > 128 || /[\x00\r\n]/.test(field)) {
      throw new Error("invalid receiver receipt token");
    }
  }
  return fields;
}
function signedDigest(fields, secretKey) {
  if (!Buffer.isBuffer(secretKey) || secretKey.length < 32) {
    throw new Error("receiver key must be at least 32 random bytes");
  }
  const h = crypto.createHmac("sha256", secretKey);
  h.update(prefix);
  for (const field of fields) {
    const bytes = Buffer.from(field, "utf8");
    const length = Buffer.allocUnsafe(4);
    length.writeUInt32BE(bytes.length, 0);
    h.update(length);
    h.update(bytes);
  }
  return h.digest("hex");
}
function oscString(text) {
  const bytes = Buffer.from(text, "utf8");
  const paddedLength = (bytes.length + 4) & ~3;
  const result = Buffer.alloc(paddedLength);
  bytes.copy(result);
  return result;
}
function encodeReceipt(receipt, key) {
  const fields = fieldsFromReceipt(receipt);
  const signature = signedDigest(fields, key);
  const packet = Buffer.concat([
    oscString(address),
    oscString("," + "s".repeat(fields.length + 1)),
    ...[...fields, signature].map(oscString),
  ]);
  if (packet.length > 4096) throw new Error("receiver acknowledgment oversized");
  return packet;
}
// This deterministic example is a cross-language test vector ONLY.
// Never use this constant in a real receiver or production configuration.
const labFixture = {
  project_id: "project-a", runtime_snapshot_id: "snapshot-a",
  machine_role_id: "ableton", companion_id: "companion-a",
  receiver_instance_id: "max4live-instrument-a", kind: "ABLETON",
  connection_generation: "12", execution_id: "exec-a",
  cue_id: "cue-7", challenge: "nonce-unique-abcdef-2026",
  sequence: "1", status: "APP_REPORTED_RECEIVED",
};
const expectedFixtureHMAC =
  "8fc1a6d833f708cdb3e7d622cf8e91f973b49e5d9d11563033d4f56b93ea556f";

function selfTest() {
  const secret = Buffer.alloc(32, 0x42);
  const fields = fieldsFromReceipt(labFixture);
  if (signedDigest(fields, secret) !== expectedFixtureHMAC) {
    throw new Error("cross-language HMAC test vector mismatch");
  }
  const packet = encodeReceipt(labFixture, secret);
  if (!packet.includes(Buffer.from(expectedFixtureHMAC, "utf8")) ||
      !packet.includes(Buffer.from(address, "utf8"))) {
    throw new Error("OSC receipt test vector not encoded");
  }
}

if (require.main === module) {
  if (process.argv[2] === "--self-test") {
    selfTest();
    process.stdout.write("receiver-ack lab encoder: OK\n");
  } else {
    // Print Base64 only: no UDP send, no transport, no secret logging.
    // Explicit lab environment secret is required to produce a packet.
    const hex = process.env.STAGECORE_LAB_RECEIVER_KEY_HEX || "";
    if (!/^(?:[0-9a-fA-F]{2}){32,}$/.test(hex)) {
      throw new Error("missing/invalid STAGECORE_LAB_RECEIVER_KEY_HEX");
    }
    const input = fs.readFileSync(0, "utf8");
    if (Buffer.byteLength(input, "utf8") > 4096) throw new Error("oversized input");
    const notice = JSON.parse(input);
    const packet = encodeReceipt(notice, Buffer.from(hex, "hex"));
    process.stdout.write(packet.toString("base64") + "\n");
  }
}
module.exports = { encodeReceipt, fieldsFromReceipt, signedDigest, labFixture };
