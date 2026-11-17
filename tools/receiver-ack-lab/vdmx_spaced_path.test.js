"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const http = require("node:http");
const { validPublishedPath, validChildName,
  encodePublishedGETPath } = require("./vdmx_oscquery_path");
const { readVDMXPublishedControl } = require("./vdmx_oscquery_readback");
const { parseArgs, getLoopbackControl } = require("./vdmx_loopback_read");
const { compareManual } = require("./vdmx_manual_compare");
const { VDMXOfflineReadbackSession } = require("./vdmx_readback_session");

const actualVDMXPath = "/OSCQUERY/Control Surface/Slider";
const encoded = "/OSCQUERY/Control%20Surface/Slider";
const env = { STAGECORE_VDMX_DISPOSABLE_TEST_PROJECT: "1" };
const fakeSnapshot = (value=0.4) => ({
  FULL_PATH:"/",CONTENTS:{
    OSCQUERY:{FULL_PATH:"/OSCQUERY",CONTENTS:{
      "Control Surface":{FULL_PATH:"/OSCQUERY/Control Surface",CONTENTS:{
        Slider:{FULL_PATH:actualVDMXPath,TYPE:"f",VALUE:[value]},
      }},
    }},
  },
});

test("real observed VDMX 6 path with interior space is valid, GET URL is encoded", () => {
  assert.equal(validPublishedPath(actualVDMXPath),true);
  assert.equal(validChildName("Control Surface"),true);
  assert.equal(encodePublishedGETPath(actualVDMXPath),encoded);
  assert.equal(parseArgs(["--port","2345","--path",actualVDMXPath],env).target,
    actualVDMXPath);
  assert.equal(validPublishedPath("/"),false);
  assert.equal(validPublishedPath("/",true),true);
});

test("unsafe raw/percent-encoded paths are rejected instead of double encoding", () => {
  for (const candidate of [
    "/OSCQUERY/Control%20Surface/Slider",
    "/OSCQUERY/Control Surface?HOST_INFO",
    "/OSCQUERY/Control Surface#fragment",
    "/OSCQUERY/../secret",
    "/OSCQUERY/./secret",
    "/OSCQUERY//Slider",
    "/OSCQUERY/ Control Surface/Slider",
    "/OSCQUERY/Control Surface /Slider",
    "/OSCQUERY/Control\nSurface/Slider",
    "/OSCQUERY/Control\tSurface/Slider",
    "/OSCQUERY/Control Surface/",
    "https://127.0.0.1:2345/OSCQUERY/Control Surface/Slider",
    "/OSCQUERY/Control\\Surface/Slider",
    "/OSCQUERY/Control+Surface/Slider",
  ]) {
    assert.equal(validPublishedPath(candidate),false,candidate);
    assert.throws(()=>encodePublishedGETPath(candidate),/invalid/,candidate);
  }
});

test("nested OSCQuery CONTENTS parser reads the published Control Surface child with spaces", () => {
  const observed = readVDMXPublishedControl(fakeSnapshot(),actualVDMXPath);
  assert.equal(observed.property,actualVDMXPath);
  assert.equal(observed.value,0.4);
  assert.equal(observed.actualVideoOutputVerified,false);
});

test("a single loopback GET encodes spaces but verifies returned raw FULL_PATH",async(t)=>{
  let requests=0;
  const srv=http.createServer((req,res)=>{
    requests++;
    assert.equal(req.method,"GET");
    assert.equal(req.url,encoded);
    res.writeHead(200,{"content-type":"application/json"});
    res.end(JSON.stringify({
      FULL_PATH:actualVDMXPath, TYPE:"f", VALUE:[0.3],
    }));
  });
  await new Promise(resolve=>srv.listen(0,"127.0.0.1",resolve));
  t.after(()=>new Promise(resolve=>srv.close(resolve)));
  const result=await getLoopbackControl({
    port:srv.address().port,target:actualVDMXPath,
  });
  assert.equal(result.published_value,0.3);
  assert.equal(result.path,actualVDMXPath);
  assert.equal(result.physical_video_output_verified,false);
  assert.equal(requests,1);
});

test("manual comparator accepts the actual VDMX path and still waits for user",async(t)=>{
  let value=0.25, requests=0;
  const srv=http.createServer((req,res)=>{
    requests++;
    assert.equal(req.url,encoded);
    res.writeHead(200,{"content-type":"application/json"});
    res.end(JSON.stringify({FULL_PATH:actualVDMXPath,TYPE:"f",VALUE:value}));
  });
  await new Promise(resolve=>srv.listen(0,"127.0.0.1",resolve));
  t.after(()=>new Promise(resolve=>srv.close(resolve)));
  const result=await compareManual({
    port:srv.address().port,target:actualVDMXPath,
  },{
    confirmed:true,
    confirm:async()=>{
      assert.equal(requests,1);
      value=0.8;
    },
  });
  assert.equal(result.requests_made,2);
  assert.equal(result.software_value_differed,true);
  assert.equal(result.target_path,actualVDMXPath);
  assert.equal(result.application_cue_acknowledged,false);
  assert.equal(result.change_caused_by_stagecore_verified,false);
});

test("spaced path rename invalidates last value without evidence of video playback",()=>{
  const ms=1_800_000_000_000;
  const scope={project_id:"p",runtime_snapshot_id:"s",
    machine_role_id:"VIDEO-VDMX",receiver_instance_id:"r",generation:7};
  const reader=new VDMXOfflineReadbackSession({
    targetPath:actualVDMXPath,now:()=>ms,
  });
  reader.bindQualified(scope);
  reader.observeSnapshot(scope,1,ms,fakeSnapshot(0.25));
  assert.equal(reader.view().published_value,0.25);
  reader.observeNotification(scope,2,ms,{
    COMMAND:"PATH_RENAMED",
    DATA:{OLD:"/OSCQUERY/Control Surface",
      NEW:"/OSCQUERY/Control Surface New"},
  });
  assert.equal(reader.view().condition,"UNKNOWN");
  assert.equal(reader.view().physical_video_output_verified,false);
});
