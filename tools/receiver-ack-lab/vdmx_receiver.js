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
  const {port, token, oscPort, queryPort, feedbackPort, feedbackSourcePort} = config;
  const ports = [port, oscPort, queryPort, feedbackPort];
  const mappings = config.mappings ?? [{inputPath: config.inputPath, queryPath: config.queryPath, feedbackPath: config.feedbackPath}];
  const localOSCPath = path => typeof path === "string" && /^\/stagecore(?:\/[A-Za-z0-9_-]+)+$/.test(path) && path.length <= 121;
  if ((feedbackSourcePort !== undefined && (!Number.isSafeInteger(feedbackSourcePort) || feedbackSourcePort < 1024 || feedbackSourcePort > 65535 || feedbackSourcePort === feedbackPort)) ||
      ports.some(p => !Number.isSafeInteger(p) || p < 1024 || p > 65535) ||
      new Set(ports).size !== ports.length || !Array.isArray(mappings) || mappings.length < 1 || mappings.length > 32 ||
      mappings.some(mapping => !mapping ||
        // A Control Surface value is not proof of media playback or a rendered frame.
        (mapping.proofKind !== undefined && mapping.proofKind !== "CONTROL_VALUE_ONLY") ||
        !validPublishedPath(mapping.queryPath) ||
        !localOSCPath(mapping.inputPath) || !localOSCPath(mapping.feedbackPath)) ||
      new Set(mappings.flatMap(mapping => [mapping.inputPath, mapping.feedbackPath])).size !== mappings.length * 2 ||
      new Set(mappings.map(mapping => mapping.queryPath)).size !== mappings.length) {
    throw new Error("invalid dedicated VDMX mapping");
  }
  const byAddress = new Map(mappings.map(mapping => [mapping.inputPath, {...mapping}]));
  const socket = dgram.createSocket("udp4");
  let pending;
  let failed = false;
  socket.on("error", error => {
    failed = true;
    pending?.reject(error);
  });
  socket.on("message", (packet, peer) => {
    if (peer.address === "127.0.0.1" &&
        (feedbackSourcePort === undefined || peer.port === feedbackSourcePort) &&
        pending?.packet.equals(packet)) pending.resolve();
  });

  function appliedFeedback(mapping, value, signal) {
    signal.throwIfAborted();
    return new Promise((resolve, reject) => {
      const abort = () => finish(signal.reason);
      function finish(err) {
        signal.removeEventListener("abort", abort);
        pending = null;
        if (err) reject(err);
        else resolve();
      }
      pending = {packet: floatOSC(mapping.feedbackPath, value), resolve: () => finish(), reject: finish};
      signal.addEventListener("abort", abort, {once: true});
      socket.send(floatOSC(mapping.inputPath, value), oscPort, "127.0.0.1", error => {
        if (error && pending?.reject === finish) finish(error);
      });
    });
  }

  function report(command, status) {
    return {status, execution_id: command.execution_id, correlation_id: command.correlation_id ?? null,
      challenge: command.application_ack.challenge};
  }
  const readControl = mapping => getLoopbackControl({port: queryPort, target: mapping.queryPath});
  const target = command => Math.fround(command.parameters.arguments[0].value);
  const processOne = createApplicationReceiver({port, token, kind: "VDMX",
    async accept(command, signal) {
      if (failed) throw new Error("VDMX feedback socket offline");
      const {address, arguments: args} = command.parameters ?? {};
      const mapping = byAddress.get(address);
      if (command.capability !== "osc.send" || !mapping || !Array.isArray(args) ||
          args.length !== 1 || args[0]?.type !== "float32" || !Number.isFinite(args[0].value) ||
          args[0].value < 0 || args[0].value > 1) throw new Error("unmapped VDMX action");
      const baseline = await readControl(mapping);
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
      const mapping = byAddress.get(command.parameters.address);
      await appliedFeedback(mapping, expected, signal);
      const current = await readControl(mapping);
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
    feedbackSourcePort: process.env.STAGECORE_VDMX_FEEDBACK_SOURCE_PORT ? Number(process.env.STAGECORE_VDMX_FEEDBACK_SOURCE_PORT) : undefined,
    mappings: process.env.STAGECORE_VDMX_MAPPINGS ? JSON.parse(process.env.STAGECORE_VDMX_MAPPINGS) : undefined,
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
