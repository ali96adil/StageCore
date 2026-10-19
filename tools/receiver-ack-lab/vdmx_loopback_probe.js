#!/usr/bin/env node
"use strict";

// Explicit ONE-SHOT, LOOPBACK-ONLY OSCQuery GET used in a disposable
// VDMX test project. No discovery, automatic polling, WebSocket, OSC
// sending, GO, MIDI, or write requests. NOT a trusted Hub device feed.
const http = require("node:http");
const { readVDMXPublishedControl } = require("./vdmx_oscquery_readback");

const MAX_RESPONSE_BYTES = 65536;
const TIMEOUT_MS = 2500;
const TARGET = /^\/[A-Za-z0-9_/-]{1,120}$/;

function validateOptions({ port, targetPath }) {
  const portText = String(port ?? "");
  if (!/^[1-9][0-9]{0,4}$/.test(portText) ||
      Number(portText) > 65535) {
    throw new Error("explicit valid TCP port required");
  }
  if (typeof targetPath !== "string" || !TARGET.test(targetPath) ||
      targetPath.endsWith("/") || targetPath.includes("//") ||
      targetPath.includes("/../") || targetPath.includes("/./")) {
    throw new Error("explicit OSCQuery control FULL_PATH required");
  }
  return { port: Number(portText), targetPath };
}

function getLocalControl({ port, targetPath, transport = http, timeoutMs = TIMEOUT_MS }) {
  const opts = validateOptions({ port, targetPath });
  if (!Number.isSafeInteger(timeoutMs) || timeoutMs < 10 || timeoutMs > 5000) {
    return Promise.reject(new Error("invalid timeout"));
  }
  return new Promise((resolve, reject) => {
    let done = false;
    let size = 0;
    const chunks = [];
    let request;
    let deadline;
    const finish = (error, result) => {
      if (done) return;
      done = true;
      clearTimeout(deadline);
      if (error) reject(error);
      else resolve(result);
    };
    // A fixed numeric IPv4 loopback host prevents arbitrary URL/host,
    // credential, hostname resolution, redirects and nonlocal targets.
    try {
      request = transport.request({
        host: "127.0.0.1",
        port: opts.port,
        method: "GET",
        path: opts.targetPath,
        headers: { Accept: "application/json", Connection: "close" },
        agent: false,
      }, (response) => {
        if (response.statusCode !== 200) {
          response.resume();
          finish(new Error("OSCQuery GET must return HTTP 200"));
          request.destroy();
          return;
        }
        const mime = String(response.headers["content-type"] || "")
          .split(";")[0].trim().toLowerCase();
        if (mime !== "application/json" && !mime.endsWith("+json")) {
          response.resume();
          finish(new Error("OSCQuery response is not JSON"));
          request.destroy();
          return;
        }
        const lengthHeader = response.headers["content-length"];
        if (lengthHeader !== undefined &&
            (!/^[0-9]+$/.test(String(lengthHeader)) ||
             Number(lengthHeader) > MAX_RESPONSE_BYTES)) {
          response.resume();
          finish(new Error("oversized OSCQuery response"));
          request.destroy();
          return;
        }
        response.on("data", (chunk) => {
          size += chunk.length;
          if (size > MAX_RESPONSE_BYTES) {
            finish(new Error("oversized OSCQuery response"));
            request.destroy();
            return;
          }
          chunks.push(chunk);
        });
        response.on("error", (error) => finish(error));
        response.on("end", () => {
          if (done) return;
          try {
            const root = JSON.parse(Buffer.concat(chunks).toString("utf8"));
            // Query exactly one node: if a server returns the whole tree
            // instead, never search for some other target implicitly.
            if (root === null || Array.isArray(root) ||
                typeof root !== "object" || root.FULL_PATH !== opts.targetPath) {
              throw new Error("OSCQuery response FULL_PATH mismatch");
            }
            const view = readVDMXPublishedControl(root, opts.targetPath);
            finish(null, {
              path: view.property,
              value: view.value,
              source: "UNAUTHENTICATED_LOCAL_OSCQUERY_HTTP_SAMPLE",
              connection_scope_verified: false,
              application_cue_acknowledged: false,
              physical_video_output_verified: false,
              commands_enabled: false,
              automated_replay: false,
            });
          } catch (error) {
            finish(error);
          }
        });
      });
    } catch (error) {
      finish(error);
      return;
    }
    deadline = setTimeout(() => {
      finish(new Error("OSCQuery GET timed out"));
      request.destroy();
    }, timeoutMs);
    request.on("error", (error) => finish(error));
    request.end();
  });
}

async function main() {
  if (process.argv.length !== 4) {
    throw new Error("usage: node vdmx_loopback_probe.js PORT /Published/Control");
  }
  const result = await getLocalControl({
    port: process.argv[2],
    targetPath: process.argv[3],
  });
  process.stdout.write(JSON.stringify(result, null, 2) + "\n");
}
if (require.main === module) {
  main().catch(() => {
    process.stderr.write(
      "Local OSCQuery readback unavailable/invalid; playback UNKNOWN\n");
    process.exitCode = 1;
  });
}
module.exports = { getLocalControl, validateOptions };
