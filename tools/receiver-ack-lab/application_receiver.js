"use strict";

// Application-side gateway client. The mapped application's own code must
// produce correlated evidence after acceptance and after application.
function createApplicationReceiver({port, token, kind, accept, apply, now = Date.now}) {
  if (!Number.isInteger(port) || port < 1024 || port > 65535 ||
      typeof token !== "string" || !/^[A-Za-z0-9_-]{32,256}$/.test(token) ||
      !["ABLETON", "VDMX"].includes(kind) || typeof accept !== "function" || typeof apply !== "function") {
    throw new Error("explicit application receiver configuration is required");
  }
  const base = "http://127.0.0.1:" + port + "/stagecore/";
  let busy = false;

  function call(endpoint, signal, report) {
    return fetch(base + endpoint, {
      headers: {authorization: "Bearer " + token, "content-type": "application/json"},
      method: report ? "POST" : "GET", body: report ? JSON.stringify(report) : undefined,
      redirect: "error", signal
    });
  }

  return async function processOne() {
    if (busy) return "BUSY";
    busy = true;
    try {
      const claim = await call("pending", AbortSignal.timeout(1000));
      if (claim.status === 404) return "IDLE";
      if (!claim.ok) throw new Error("application claim rejected");
      const command = await claim.json();
      const {application_ack: ack, receiver_deadline_ms: deadline} = command;
      const timeLeft = deadline - now();
      if (ack?.scope?.kind !== kind || ack.execution_id !== command.execution_id ||
          !Number.isSafeInteger(deadline) || timeLeft <= 0 || timeLeft > 12000) {
        throw new Error("application claim identity or deadline invalid");
      }
      const report = {...ack.scope, execution_id: ack.execution_id, cue_id: ack.cue_id,
        challenge: ack.challenge, correlation_id: command.correlation_id ?? null};
      const signal = AbortSignal.timeout(timeLeft);
      function verifyEvidence(evidence, status) {
        if (signal.aborted || now() >= deadline || evidence?.status !== status ||
            evidence.execution_id !== report.execution_id ||
            evidence.correlation_id !== report.correlation_id || evidence.challenge !== report.challenge) {
          throw new Error("application evidence missing, expired or mismatched");
        }
      }
      async function sendEvidence(handler, status) {
        verifyEvidence(await handler(command, signal), status);
        if (!(await call("receipt", signal, {...report, status})).ok) {
          throw new Error("application evidence rejected");
        }
      }
      await sendEvidence(accept, "APP_REPORTED_RECEIVED");
      if (signal.aborted || now() >= deadline) throw new Error("application claim expired");
      await sendEvidence(apply, "APP_REPORTED_APPLIED");
      return "APPLIED";
    } finally {
      busy = false;
    }
  };
}

module.exports = { createApplicationReceiver };
