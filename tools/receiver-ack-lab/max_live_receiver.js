"use strict";

const { randomUUID } = require("node:crypto");
const { createApplicationReceiver } = require("./application_receiver");

function registerLiveReceiver(max, {port, token, trackName, allowArm = false, allowTransport = false, allowClip = false}) {
  if (typeof trackName !== "string" || !trackName.trim() || trackName.length > 80) {
    throw new Error("configure the dedicated Live track name");
  }
  let pending;
  let binding;
  max.addHandler("stagecore_live_result", (ticket, ok, id) => {
    if (ticket === pending?.ticket) pending.finish(ok === 1 && Number.isSafeInteger(id) && id > 0 ? id : null);
  });

  function requestLive(phase, command, signal, operation, value, trackID) {
    signal.throwIfAborted();
    const ticket = randomUUID();
    return new Promise((resolve, reject) => {
      function finish(id) {
        if (pending?.ticket !== ticket) return;
        pending = null;
        signal.removeEventListener("abort", abort);
        if (id === null) reject(new Error("Live API could not confirm the mapped operation"));
        else resolve(id);
      }
      const abort = () => finish(null);
      pending = {ticket, finish};
      signal.addEventListener("abort", abort, {once: true});
      Promise.resolve().then(() => max.outlet("stagecore_live", phase, ticket, trackName,
        operation, value, trackID, "t" + command.receiver_deadline_ms)).catch(abort);
    });
  }

  function receipt(command, status) {
    return {status, execution_id: command.execution_id, correlation_id: command.correlation_id ?? null,
      challenge: command.application_ack.challenge};
  }

  const processOne = createApplicationReceiver({port, token, kind: "ABLETON",
    async accept(command, signal) {
      binding = null;
      const {address, arguments: args} = command.parameters ?? {};
      const operations = new Map([["/stagecore/ableton/mute", "mute"], ["/stagecore/ableton/solo", "solo"]]);
      if (allowArm) operations.set("/stagecore/ableton/arm", "arm");
      if (allowTransport) operations.set("/stagecore/ableton/transport", "transport");
      if (allowClip) operations.set("/stagecore/ableton/clip/fire", "clip");
      const operation = operations.get(address);
      if (command.capability !== "osc.send" || !operation ||
          !Array.isArray(args) || args.length !== 1 || args[0]?.type !== "int32" ||
          !(operation === "clip"
            ? Number.isInteger(args[0].value) && args[0].value >= 0 && args[0].value <= 255
            : [0, 1].includes(args[0].value))) throw new Error("unmapped Ableton action");
      const value = args[0].value;
      binding = {command, operation, value, trackID: await requestLive("check", command, signal, operation, value, 0)};
      return receipt(command, "APP_REPORTED_RECEIVED");
    },
    async apply(command, signal) {
      if (binding?.command !== command) throw new Error("missing Live track binding");
      const {operation, value, trackID} = binding;
      binding = null;
      if (await requestLive("apply", command, signal, operation, value, trackID) !== trackID) {
        throw new Error("Live track changed during execution");
      }
      return receipt(command, "APP_REPORTED_APPLIED");
    }
  });
  max.addHandler("poll", async () => {
    try { await processOne(); } catch (_) { max.post("StageCore Live: no confirmed application result"); }
  });
}

if (require.main === module) {
  if (process.env.STAGECORE_LIVE_RECEIVER !== "1") throw new Error("StageCore Live receiver is disabled");
  registerLiveReceiver(require("max-api"), {
    port: Number(process.env.STAGECORE_ACK_GATEWAY_PORT),
    token: process.env.STAGECORE_ACK_APPLICATION_TOKEN,
    trackName: process.env.STAGECORE_LIVE_TRACK,
    allowArm: process.env.STAGECORE_LIVE_ALLOW_ARM === "1",
    allowTransport: process.env.STAGECORE_LIVE_ALLOW_TRANSPORT === "1",
    allowClip: process.env.STAGECORE_LIVE_ALLOW_CLIP === "1"
  });
}

module.exports = { registerLiveReceiver };
