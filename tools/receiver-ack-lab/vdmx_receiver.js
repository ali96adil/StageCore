"use strict";

const dgram = require("node:dgram");
const {once} = require("node:events");
const {setTimeout: sleep} = require("node:timers/promises");
const {createApplicationReceiver} = require("./application_receiver");
const {getLoopbackControl} = require("./vdmx_loopback_read");
const {validPublishedPath} = require("./vdmx_oscquery_path");

function floatOSC(address, value) {
  const addressBytes = Buffer.from(address);
  const offset = (addressBytes.length + 4) & ~3;
  const packet = Buffer.alloc(offset + 8);
  addressBytes.copy(packet);
  packet.write(",f", offset);
  packet.writeFloatBE(value, offset + 4);
  return packet;
}

async function openVDMXReceiver(config) {
  const {port, token, oscPort, queryPort, feedbackPort, inputPath, queryPath, feedbackPath} = config;
  const ports = [port, oscPort, queryPort, feedbackPort];
  if (ports.some(p => !Number.isSafeInteger(p) || p < 1024 || p > 65535) ||
      new Set(ports).size !== ports.length || !validPublishedPath(queryPath) ||
      ![inputPath, feedbackPath].every(path => typeof path === "string" && /^\/stagecore(?:\/[A-Za-z0-9_-]+)+$/.test(path) && path.length <= 121) ||
      inputPath === feedbackPath) throw new Error("invalid dedicated VDMX mapping");
  const socket = dgram.createSocket("udp4");
  let pending;
  let failed = false;
  socket.on("error", error => {
    failed = true;
    pending?.reject(error);
  });
  socket.on("message", (packet, peer) => {
    if (peer.address === "127.0.0.1" && pending?.packet.equals(packet)) pending.resolve();
  });

  function appliedFeedback(value, signal) {
    signal.throwIfAborted();
    return new Promise((resolve, reject) => {
      const abort = () => finish(signal.reason);
      function finish(err) {
        signal.removeEventListener("abort", abort);
        pending = null;
        if (err) reject(err);
        else resolve();
      }
      pending = {packet: floatOSC(feedbackPath, value), resolve: () => finish(), reject: finish};
      signal.addEventListener("abort", abort, {once: true});
      socket.send(floatOSC(inputPath, value), oscPort, "127.0.0.1", error => {
        if (error && pending?.reject === finish) finish(error);
      });
    });
  }

  function report(command, status) {
    return {status, execution_id: command.execution_id, correlation_id: command.correlation_id ?? null,
      challenge: command.application_ack.challenge};
  }
  const readControl = () => getLoopbackControl({port: queryPort, target: queryPath});
  const target = command => Math.fround(command.parameters.arguments[0].value);
  const processOne = createApplicationReceiver({port, token, kind: "VDMX",
    async accept(command, signal) {
      if (failed) throw new Error("VDMX feedback socket offline");
      const {address, arguments: args} = command.parameters ?? {};
      if (command.capability !== "osc.send" || address !== inputPath || !Array.isArray(args) ||
          args.length !== 1 || args[0]?.type !== "float32" || !Number.isFinite(args[0].value) ||
          args[0].value < 0 || args[0].value > 1) throw new Error("unmapped VDMX action");
      const baseline = await readControl();
      signal.throwIfAborted();
      if (!Number.isFinite(baseline.published_value) || Math.fround(baseline.published_value) === target(command)) {
        throw new Error("no distinguishable VDMX control transition");
      }
      return report(command, "APP_REPORTED_RECEIVED");
    },
    async apply(command, signal) {
      if (failed) throw new Error("VDMX feedback socket offline");
      const expected = target(command);
      if (Date.now() >= command.receiver_deadline_ms) throw new Error("VDMX claim expired");
      await appliedFeedback(expected, signal);
      const current = await readControl();
      signal.throwIfAborted();
      if (!Number.isFinite(current.published_value) || Math.fround(current.published_value) !== expected) {
        throw new Error("VDMX control readback did not confirm the change");
      }
      return report(command, "APP_REPORTED_APPLIED");
    }
  });
  try {
    socket.bind(feedbackPort, "127.0.0.1");
    await once(socket, "listening");
  } catch (error) {
    socket.close();
    throw error;
  }
  return {processOne, close() {
    failed = true;
    pending?.reject(new Error("VDMX receiver closed"));
    socket.close();
  }};
}

async function main() {
  if (process.env.STAGECORE_VDMX_RECEIVER !== "1") throw new Error("VDMX receiver is disabled");
  const receiver = await openVDMXReceiver({
    port: Number(process.env.STAGECORE_ACK_GATEWAY_PORT),
    token: process.env.STAGECORE_ACK_APPLICATION_TOKEN,
    oscPort: Number(process.env.STAGECORE_VDMX_OSC_PORT),
    queryPort: Number(process.env.STAGECORE_VDMX_QUERY_PORT),
    feedbackPort: Number(process.env.STAGECORE_VDMX_FEEDBACK_PORT),
    inputPath: process.env.STAGECORE_VDMX_INPUT,
    queryPath: process.env.STAGECORE_VDMX_QUERY_PATH,
    feedbackPath: process.env.STAGECORE_VDMX_FEEDBACK
  });
  try {
    for (;;) {
      try { await receiver.processOne(); } catch (_) { process.stderr.write("StageCore VDMX: no confirmed application result\n"); }
      await sleep(125);
    }
  } finally {
    receiver.close();
  }
}
if (require.main === module) main().catch(() => { process.stderr.write("StageCore VDMX receiver unavailable\n"); process.exitCode = 1; });
module.exports = {openVDMXReceiver, floatOSC};
