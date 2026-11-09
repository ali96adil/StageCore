"use strict";

const http = require("node:http");
const crypto = require("node:crypto");
const fs = require("node:fs");
const { encodeReceipt, fieldsFromReceipt } = require("./receiver_ack");

const scopeKeys = ["project_id", "runtime_snapshot_id", "machine_role_id", "companion_id",
  "receiver_instance_id", "kind", "connection_generation"];
const pinnedKeys = scopeKeys.filter(key => !["runtime_snapshot_id", "connection_generation"].includes(key));

function authorized(req, token) {
  const actual = Buffer.from(req.headers.authorization || "");
  const expected = Buffer.from("Bearer " + token);
  return actual.length === expected.length && crypto.timingSafeEqual(actual, expected);
}

async function readObject(req) {
  let size = 0;
  const chunks = [];
  for await (const chunk of req) {
    size += chunk.length;
    if (size > 48 * 1024) throw new Error("request too large");
    chunks.push(chunk);
  }
  const data = JSON.parse(Buffer.concat(chunks).toString("utf8"));
  if (!data || typeof data !== "object" || Array.isArray(data)) throw new Error("object required");
  return data;
}

function createReceiverGateway({ companionToken, applicationToken, key, scope, now = Date.now }) {
  const validToken = token => typeof token === "string" && /^[A-Za-z0-9_-]{32,256}$/.test(token);
  if (!validToken(companionToken) || !validToken(applicationToken) || applicationToken === companionToken ||
      !Buffer.isBuffer(key) || key.length < 32 || key.length > 128 || !scope ||
      !pinnedKeys.every(field => typeof scope[field] === "string" && scope[field].length > 0) ||
      !["ABLETON", "VDMX"].includes(scope.kind)) {
    throw new Error("private receiver configuration is invalid");
  }
  let pending;
  const seen = new Set();
  const json = (res, code, body) => {
    res.writeHead(code, {"content-type":"application/json", "cache-control":"no-store"});
    res.end(JSON.stringify(body));
  };
  function clear(item) {
    if (pending !== item) return false;
    clearTimeout(item.timer);
    pending = undefined;
    return true;
  }
  return http.createServer({requestTimeout: 15000}, async (req, res) => {
    try {
      if (req.socket.remoteAddress !== "127.0.0.1") return json(res, 403, {});
      if (req.method === "POST" && req.url === "/stagecore/execute") {
        if (!authorized(req, companionToken)) return json(res, 401, {});
        const command = await readObject(req);
        const dispatch = command.application_ack;
        const runtimeScope = dispatch?.scope;
        const expires = Date.parse(dispatch?.expires_at);
        const current = now();
        if (command.type !== "execution.request" || command.schema_version !== 1 ||
            !["osc.send", "midi.send"].includes(command.capability) || !runtimeScope ||
            !pinnedKeys.every(field => scope[field] === runtimeScope[field]) ||
            !Number.isSafeInteger(runtimeScope.connection_generation) || runtimeScope.connection_generation <= 0 ||
            command.execution_id !== dispatch.execution_id || command.machine_role_id !== runtimeScope.machine_role_id ||
            command.runtime_snapshot_id !== runtimeScope.runtime_snapshot_id ||
            !/^[0-9a-f]{64}$/.test(dispatch.challenge) || !Number.isFinite(expires) ||
            expires <= current || expires - current > 31000 ||
            (command.correlation_id != null && typeof command.correlation_id !== "string")) {
          return json(res, 400, {});
        }
        if (pending || seen.has(dispatch.execution_id) || seen.size >= 8192) return json(res, 409, {});
        const receipt = Object.fromEntries(scopeKeys.map(field => [field, runtimeScope[field]]));
        Object.assign(receipt, {execution_id: dispatch.execution_id, cue_id: dispatch.cue_id,
          challenge: dispatch.challenge, sequence: "1", status: "APP_REPORTED_RECEIVED"});
        fieldsFromReceipt(receipt);
        seen.add(dispatch.execution_id);
        const item = {command, receipt, res, claimed: false, received: null};
        const timeout = Math.min(expires - current, command.timeout_ms > 0 ? command.timeout_ms : 12000, 12000);
        item.timer = setTimeout(() => { if (clear(item)) json(res, 504, {}); }, timeout);
        pending = item;
        res.on("close", () => clear(item));
        return;
      }
      if (!authorized(req, applicationToken)) return json(res, 401, {});
      if (req.method === "GET" && req.url === "/stagecore/pending") {
        if (!pending || pending.claimed) return json(res, 404, {});
        pending.claimed = true;
        return json(res, 200, pending.command);
      }
      if (req.method === "POST" && req.url === "/stagecore/receipt") {
        const report = await readObject(req);
        const item = pending;
        if (!item?.claimed || !scopeKeys.every(field => report[field] === item.receipt[field]) ||
            report.execution_id !== item.receipt.execution_id || report.cue_id !== item.receipt.cue_id ||
            report.challenge !== item.receipt.challenge ||
            report.correlation_id !== (item.command.correlation_id ?? null)) {
          return json(res, 409, {});
        }
        if (report.status === "APP_REPORTED_RECEIVED" && !item.received) {
          item.received = encodeReceipt(item.receipt, key);
          return json(res, 200, {});
        }
        if (report.status !== "APP_REPORTED_APPLIED" || !item.received) return json(res, 409, {});
        const applied = encodeReceipt({...item.receipt, sequence: "2", status: report.status}, key);
        if (!clear(item)) return json(res, 409, {});
        json(item.res, 200, {execution_id: item.receipt.execution_id,
          correlation_id: item.command.correlation_id ?? null,
          signed_application_receipts: [item.received, applied].map(packet => packet.toString("base64"))});
        return json(res, 200, {});
      }
      return json(res, 404, {});
    } catch (_) {
      if (!res.headersSent) json(res, 400, {});
    }
  });
}

if (require.main === module) {
  try {
    const path = process.argv[2];
    const file = fs.openSync(path, fs.constants.O_RDONLY | fs.constants.O_NOFOLLOW);
    let cfg;
    try {
      const info = fs.fstatSync(file);
      if (!info.isFile() || (info.mode & 0o077) !== 0 || info.size > 4096) throw new Error("private config required");
      cfg = JSON.parse(fs.readFileSync(file, "utf8"));
    } finally {
      fs.closeSync(file);
    }
    if (!Number.isInteger(cfg.port) || cfg.port < 1024 || cfg.port > 65535) throw new Error("invalid port");
    createReceiverGateway({...cfg, key: Buffer.from(cfg.key, "base64")}).listen(cfg.port, "127.0.0.1");
  } catch (_) {
    process.stderr.write("StageCore receiver gateway requires a valid private configuration\n");
    process.exitCode = 1;
  }
}

module.exports = { createReceiverGateway };
