"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const vm = require("node:vm");
const fs = require("node:fs");
const http = require("node:http");
const dgram = require("node:dgram");
const {once} = require("node:events");
const {setTimeout: sleep} = require("node:timers/promises");
const {registerLiveReceiver} = require("./max_live_receiver");
const {openVDMXReceiver, floatOSC} = require("./vdmx_receiver");
const {createReceiverGateway} = require("./receiver_gateway");

const token = "a".repeat(32), companion = "c".repeat(32);
async function harness(t, kind) {
  const scope = {project_id:"p", runtime_snapshot_id:"s", machine_role_id:"r", companion_id:"c",
    receiver_instance_id:"i", kind, connection_generation:5};
  const gateway = createReceiverGateway({key: Buffer.alloc(32, 35), companionToken:companion, applicationToken:token, scope});
  gateway.listen(0, "127.0.0.1");
  await once(gateway, "listening");
  t.after(() => {gateway.closeAllConnections(); gateway.close();});
  const port = gateway.address().port;
  function dispatch(id, address, argument, timeout = 2000) {
    return fetch("http://127.0.0.1:" + port + "/stagecore/execute", {
      method:"POST", headers:{authorization:"Bearer " + companion, "content-type":"application/json"},
      body:JSON.stringify({type:"execution.request",schema_version:1,execution_id:id,correlation_id:"corr",
        machine_role_id:"r",runtime_snapshot_id:"s",capability:"osc.send",timeout_ms:timeout,
        parameters:{address,arguments:[argument]},
        application_ack:{scope,execution_id:id,cue_id:"cue",challenge:"b".repeat(64),expires_at:new Date(Date.now()+8000).toISOString()}})
    });
  }
  return {port, dispatch};
}
async function pumpUntil(poll, completion) {
  let result;
  completion.then(response => {result = response;});
  const end = Date.now() + 4000;
  while (!result && Date.now() < end) {
    await poll();
    await sleep(15);
  }
  assert.ok(result, "gateway response missing");
  return result;
}

test("Live receiver runs the patch's Live API script and fences mismatched replies and track changes", async t => {
  const {port, dispatch} = await harness(t, "ABLETON");
  const script = fs.readFileSync(__dirname + "/stagecore_live_track.js", "utf8");
  const patch = JSON.parse(fs.readFileSync(__dirname + "/StageCore-Live-Receiver.maxpat", "utf8")).patcher;
  assert.ok(patch.boxes.some(({box}) => box.text === "node.script max_live_receiver.js @autostart 0"));
  assert.deepEqual(patch.lines.slice(-3).map(({patchline}) => [patchline.source[0], patchline.destination[0]]),
    [["node", "defer"], ["defer", "js"], ["js", "node"]]);
  let value = 0, writes = 0, trackID = 7, rebind = false;
  const handlers = new Map();
  const context = vm.createContext({autowatch:0, inlets:1, outlets:1,
    LiveAPI: class {
      constructor(_, path) { assert.equal(path, "this_device canonical_parent"); this.id = trackID; this.type = "Track"; }
      get(property) { return property === "name" ? ["StageCore Test"] : [value]; }
      set(property, next) { assert.equal(property, "mute"); writes++; value = next; }
    },
    outlet(_index, handler, ...args) {handlers.get(handler)(...args);}
  });
  vm.runInContext(script, context);
  registerLiveReceiver({
    addHandler(name, fn) {handlers.set(name, fn);},
    async outlet(name, phase, ticket, ...args) {
      handlers.get("stagecore_live_result")("wrong-ticket", 1, 999);
      if (phase === "apply") {
        await sleep(20);
        if (rebind) trackID++;
      }
      context[name](phase, ticket, ...args);
    },
    post() {}
  }, {port, token, trackName:"StageCore Test"});
  const address = "/stagecore/ableton/mute";
  const mute = {type:"int32", value:1};
  assert.equal((await pumpUntil(handlers.get("poll"), dispatch("one", address, mute))).status, 200);
  assert.equal(value, 1);
  assert.equal((await dispatch("one", address, mute)).status, 409);
  rebind = true;
  assert.equal((await pumpUntil(handlers.get("poll"), dispatch("two", address, {...mute, value:0}, 350))).status, 504);
  assert.equal(writes, 1);
  rebind = false;
  assert.equal((await pumpUntil(handlers.get("poll"), dispatch("three", address, {...mute, value:0}))).status, 200);
  assert.equal(writes, 2);
});

test("VDMX receiver needs the application event and readback, then reconnect accepts only a new execution", async t => {
  const {port, dispatch} = await harness(t, "VDMX");
  const inputPath = "/stagecore/test/input", feedbackPath = "/stagecore/test/applied";
  const queryPath = "/OSCQUERY/Control Surface/StageCore Test";
  let value = 0, mode = "missing", writes = 0;
  const query = http.createServer((req, res) => {
    assert.equal(decodeURI(req.url), queryPath);
    res.writeHead(200, {"content-type":"application/json"});
    res.end(JSON.stringify({FULL_PATH:queryPath,TYPE:"f",VALUE:[value]}));
  });
  query.listen(0, "127.0.0.1"); await once(query, "listening");
  t.after(() => query.close());
  const udp = dgram.createSocket("udp4"), reservation = dgram.createSocket("udp4");
  udp.bind(0, "127.0.0.1"); await once(udp, "listening");
  reservation.bind(0, "127.0.0.1"); await once(reservation, "listening");
  const feedbackPort = reservation.address().port;
  await new Promise(resolve => reservation.close(resolve));
  udp.on("message", async packet => {
    writes++;
    const next = packet.readFloatBE(packet.length - 4);
    assert.deepEqual(packet, floatOSC(inputPath, next));
    if (mode === "missing") return;
    await sleep(20);
    if (mode === "correct") value = next;
    udp.send(floatOSC(feedbackPath, next), feedbackPort, "127.0.0.1");
  });
  t.after(() => udp.close());
  const config = {port, token, oscPort:udp.address().port, queryPort:query.address().port,
    feedbackPort, inputPath, queryPath, feedbackPath};
  let receiver = await openVDMXReceiver(config);
  t.after(() => receiver.close());
  const send = (id, next, timeout) => dispatch(id, inputPath, {type:"float32",value:next}, timeout);
  const poll = async () => {try { await receiver.processOne(); } catch (_) {}};
  assert.equal((await pumpUntil(poll, send("offline", 0.5, 350))).status, 504);
  receiver.close();
  receiver = await openVDMXReceiver(config);
  assert.equal((await send("offline", 0.5)).status, 409);
  mode = "wrong-state";
  assert.equal((await pumpUntil(poll, send("wrong-state", 0.5, 350))).status, 504);
  mode = "correct";
  assert.equal((await pumpUntil(poll, send("applied", 0.5))).status, 200);
  assert.equal(writes, 3);
  assert.equal((await pumpUntil(poll, send("already-set", 0.5, 350))).status, 504);
  assert.equal(writes, 3);
});
