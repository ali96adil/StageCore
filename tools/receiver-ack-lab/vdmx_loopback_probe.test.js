"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const http = require("node:http");
const { getLocalControl, validateOptions } = require("./vdmx_loopback_probe");

const property="/StageCore_Lab/layer_opacity";
function tree(value=0.625) {
  return {FULL_PATH:property,TYPE:"f",ACCESS:1,VALUE:[value]};
}
async function localServer(handler) {
  const server=http.createServer(handler);
  await new Promise((resolve,reject)=>{
    server.once("error",reject);
    server.listen(0,"127.0.0.1",resolve);
  });
  return {
    port:server.address().port,
    close:()=>new Promise((resolve,reject)=>
      server.close((err)=>err?reject(err):resolve())),
  };
}
test("loopback-only VDMX test probe issues exactly one read-only GET",async()=>{
  let requests=0;
  const server=await localServer((req,res)=>{
    requests++;
    assert.equal(req.method,"GET");
    assert.equal(req.url,property);
    assert.equal(req.socket.localAddress,"127.0.0.1");
    assert.equal(req.headers.accept,"application/json");
    res.writeHead(200,{"content-type":"application/json"});
    res.end(JSON.stringify(tree()));
  });
  try {
    const view=await getLocalControl({port:server.port,targetPath:property});
    assert.equal(requests,1);
    assert.equal(view.path,property);
    assert.equal(view.value,0.625);
    assert.equal(view.source,"UNAUTHENTICATED_LOCAL_OSCQUERY_HTTP_SAMPLE");
    for(const k of ["connection_scope_verified","application_cue_acknowledged",
      "physical_video_output_verified","commands_enabled","automated_replay"]) {
      assert.equal(view[k],false,k);
    }
  } finally {await server.close()}
});
test("read-only HTTP probe never follows redirects to arbitrary destinations",async()=>{
  let requests=0;
  const server=await localServer((req,res)=>{
    requests++;
    res.writeHead(302,{location:"http://example.com:80/remote"});
    res.end();
  });
  try {
    await assert.rejects(()=>getLocalControl({
      port:server.port,targetPath:property}),/HTTP 200/);
    assert.equal(requests,1);
  } finally {await server.close()}
});
test("unknown or write-only published controls cannot be read as execution",async()=>{
  const server=await localServer((req,res)=>{
    res.writeHead(200,{"content-type":"application/json"});
    const bad=tree(0);
    bad.ACCESS=2;
    res.end(JSON.stringify(bad));
  });
  try {
    await assert.rejects(()=>getLocalControl({
      port:server.port,targetPath:property}),/readable/);
  } finally {await server.close()}
});
test("GET response cannot substitute a different OSCQuery path",async()=>{
  const server=await localServer((req,res)=>{
    res.writeHead(200,{"content-type":"application/json"});
    res.end(JSON.stringify({...tree(),FULL_PATH:"/OtherProject/layer_opacity"}));
  });
  try {
    await assert.rejects(()=>getLocalControl({
      port:server.port,targetPath:property}),/FULL_PATH mismatch/);
  } finally {await server.close()}
});
test("wrong media type and malformed JSON fail closed",async()=>{
  let wrong=false;
  const server=await localServer((req,res)=>{
    res.writeHead(200,{"content-type":wrong?"application/json":"text/html"});
    res.end(wrong?"{malformed":"<html>not-JSON</html>");
  });
  try {
    await assert.rejects(()=>getLocalControl({
      port:server.port,targetPath:property}),/not JSON/);
    wrong=true;
    await assert.rejects(()=>getLocalControl({
      port:server.port,targetPath:property}),/JSON/);
  }finally{await server.close()}
});
test("oversized OSCQuery response is rejected without reading it all",async()=>{
  const server=await localServer((req,res)=>{
    res.writeHead(200,{"content-type":"application/json"});
    res.end(JSON.stringify({...tree(),EXTRA:"x".repeat(66000)}));
  });
  try{
    await assert.rejects(()=>getLocalControl({
      port:server.port,targetPath:property}),/oversized/);
  }finally{await server.close()}
});
test("single GET deadline bounds stalled HTTP connection",async()=>{
  const server=await localServer((req,res)=>{
    res.writeHead(200,{"content-type":"application/json"});
    res.write("{");
    // Deliberately no response end: the CLI must time out and close.
  });
  try {
    await assert.rejects(()=>getLocalControl({
      port:server.port,targetPath:property,timeoutMs:60}),/timed out/);
  }finally{await server.close()}
});
test("CLI refuses arbitrary URL, DNS, credentials, query, fragments and invalid port",()=>{
  for(const bad of [
    "http://127.0.0.1:8000/x","//evil.local/x","/Some?VALUE","/Some#fragment",
    "/../config","/Some/../../x","/Good//Bad","/Trailing/","/x;rm -rf",
  ]){
    assert.throws(()=>validateOptions({port:12345,targetPath:bad}),/FULL_PATH/);
  }
  for(const bad of ["","0","65536","123456","-1","http://127.0.0.1:3","localhost"]) {
    assert.throws(()=>validateOptions({port:bad,targetPath:property}),/port/);
  }
  const opts=validateOptions({port:"7841",targetPath:property});
  assert.deepEqual(opts,{port:7841,targetPath:property});
});
test("no device command, OSC send, open WebSocket or automatic polling path",()=>{
  const source=require("node:fs").readFileSync(
    require("node:path").join(__dirname,"vdmx_loopback_probe.js"),"utf8");
  for(const disallowed of ["POST","PUT","DELETE","fetch(", "WebSocket(", "setInterval(",
    "udp.send(", "dgram.createSocket(", "stagecore/go"]) {
    assert.equal(source.includes(disallowed),false,disallowed);
  }
});
