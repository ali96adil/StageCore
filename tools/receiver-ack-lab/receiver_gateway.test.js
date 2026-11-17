"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const { once } = require("node:events");
const { createReceiverGateway } = require("./receiver_gateway");
const { encodeReceipt } = require("./receiver_ack");

for (const kind of ["ABLETON", "VDMX"]) {
  test(kind + " receipt requires explicit app evidence and rejects replay", async t => {
    const key = Buffer.alloc(32, 35);
    const companionToken = "c".repeat(32);
    const applicationToken = "a".repeat(32);
    const scope = {project_id:"p", runtime_snapshot_id:"s", machine_role_id:"r", companion_id:"c",
      receiver_instance_id:"i", kind, connection_generation:5};
    const server = createReceiverGateway({key, companionToken, applicationToken, scope});
    server.listen(0, "127.0.0.1");
    await once(server, "listening");
    t.after(() => { server.closeAllConnections(); server.close(); });
    const base = "http://127.0.0.1:" + server.address().port;
    const call = (endpoint, token, body) => fetch(base + "/stagecore/" + endpoint, {
      headers: {authorization:"Bearer " + token, "content-type":"application/json"},
      method: body ? "POST" : "GET", body: body ? JSON.stringify(body) : undefined
    });
    const request = {type:"execution.request",schema_version:1,execution_id:"e",correlation_id:"corr",
      machine_role_id:"r",runtime_snapshot_id:"s",capability:"osc.send",parameters:{address:"/go"},timeout_ms:3000,
      application_ack:{scope,execution_id:"e",cue_id:"cue",challenge:"b".repeat(64),expires_at:new Date(Date.now()+8000).toISOString()}};
    assert.equal((await call("execute", applicationToken, request)).status, 401);
    const completion = call("execute", companionToken, request);
    let next;
    for (let i = 0; i < 40; i++) {
      next = await call("pending", applicationToken);
      if (next.status === 200) break;
      await new Promise(resolve => setTimeout(resolve, 5));
    }
    assert.equal(next.status, 200);
    assert.equal((await next.json()).correlation_id, "corr");
    assert.equal((await call("pending", applicationToken)).status, 404);
    const report = {...scope,execution_id:"e",cue_id:"cue",challenge:"b".repeat(64),correlation_id:"corr"};
    assert.equal((await call("receipt", applicationToken, {...report,status:"APP_REPORTED_APPLIED"})).status, 409);
    assert.equal((await call("receipt", applicationToken, {...report,project_id:"other",status:"APP_REPORTED_RECEIVED"})).status, 409);
    assert.equal((await call("receipt", applicationToken, {...report,correlation_id:"wrong",status:"APP_REPORTED_RECEIVED"})).status, 409);
    assert.equal((await call("receipt", applicationToken, {...report,status:"APP_REPORTED_RECEIVED"})).status, 200);
    assert.equal((await call("receipt", applicationToken, {...report,status:"APP_REPORTED_APPLIED"})).status, 200);
    const response = await completion;
    assert.equal(response.status, 200);
    const result = await response.json();
    assert.deepEqual(result.signed_application_receipts, ["APP_REPORTED_RECEIVED","APP_REPORTED_APPLIED"]
      .map((status,index) => encodeReceipt({...report, status, sequence:String(index+1)},key).toString("base64")));
    assert.equal((await call("execute", companionToken, request)).status, 409);
    assert.equal((await call("receipt", applicationToken, {...report,status:"APP_REPORTED_APPLIED"})).status, 409);
  });
}

test("application client requires correlated callback evidence before completion", async t => {
  const { createApplicationReceiver } = require("./application_receiver");
  const companionToken = "c".repeat(32);
  const token = "a".repeat(32);
  const scope = {project_id:"p", runtime_snapshot_id:"s", machine_role_id:"r", companion_id:"c",
    receiver_instance_id:"i", kind:"VDMX", connection_generation:5};
  let clock = Date.now();
  const now = () => clock;
  const server = createReceiverGateway({key: Buffer.alloc(32, 35), companionToken, applicationToken: token, scope, now});
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => { server.closeAllConnections(); server.close(); });
  const port = server.address().port;
  const start = id => fetch("http://127.0.0.1:" + port + "/stagecore/execute", {
    method: "POST", headers: {authorization:"Bearer " + companionToken, "content-type":"application/json"},
    body: JSON.stringify({type:"execution.request",schema_version:1,execution_id:id,correlation_id:"corr",
      machine_role_id:"r",runtime_snapshot_id:"s",capability:"osc.send",parameters:{address:"/go"},timeout_ms:3000,
      application_ack:{scope,execution_id:id,cue_id:"cue",challenge:"b".repeat(64),expires_at:new Date(now()+8000).toISOString()}})
  });
  async function next(processOne) {
    for (let i = 0; i < 40; i++) {
      const result = await processOne();
      if (result !== "IDLE") return result;
      await new Promise(resolve => setTimeout(resolve, 5));
    }
    throw new Error("claim never became available");
  }
  const evidence = (command, status) => ({
    status, execution_id: command.execution_id, correlation_id: command.correlation_id,
    challenge: command.application_ack.challenge
  });
  let outputCalls = 0;
  const config = {port, token, kind:"VDMX", now, accept: async command => evidence(command, "APP_REPORTED_RECEIVED")};
  const transportOnly = createApplicationReceiver({...config, apply: async () => {
    outputCalls++;
    return true;
  }});
  const rejected = start("transport-only");
  await assert.rejects(next(transportOnly), /evidence missing/);
  clock += 3001;
  assert.equal(await transportOnly(), "IDLE");
  assert.equal((await rejected).status, 504);
  assert.equal((await start("transport-only")).status, 409);
  assert.equal(outputCalls, 1);

  const verified = createApplicationReceiver({...config, apply: async command => {
    outputCalls++;
    return evidence(command, "APP_REPORTED_APPLIED");
  }});
  const completed = start("verified");
  assert.equal(await next(verified), "APPLIED");
  assert.equal((await completed).status, 200);
  assert.equal(outputCalls, 2);
});
