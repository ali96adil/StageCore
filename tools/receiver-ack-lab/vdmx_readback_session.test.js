"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const { VDMXOfflineReadbackSession } = require("./vdmx_readback_session");

const target="/StageCore_Lab/layer_opacity";
function fixture() {
  let nowMs=1_800_000_000_000;
  const now=()=>nowMs;
  const scope={project_id:"project-a",runtime_snapshot_id:"published-1",
    machine_role_id:"VIDEO-VDMX",receiver_instance_id:"vdmx-instance-a",
    generation:42};
  const reader=new VDMXOfflineReadbackSession({targetPath:target,now,maxAgeMs:5000});
  const snapshot=(value=0.5)=>({
    FULL_PATH:"/",CONTENTS:{StageCore_Lab:{
      FULL_PATH:"/StageCore_Lab",CONTENTS:{layer_opacity:{
        FULL_PATH:target,ACCESS:3,TYPE:"f",VALUE:[value],
      }},
    }},
  });
  return {reader,scope,snapshot,advance:(delta)=>{nowMs+=delta;},now};
}
function begin(f) {
  f.reader.bindQualified(f.scope);
  const first=f.reader.observeSnapshot(f.scope,1,f.now(),f.snapshot(0.5));
  assert.equal(first.published_value,0.5);
  assert.equal(first.application_receipt_confirmed,false);
  assert.equal(first.authenticated_receiver,false);
  assert.equal(first.physical_video_output_verified,false);
  assert.equal(first.commands_enabled,false);
  return first;
}
test("published OSCQuery VALUE is a read-only report, never ACK or physical video proof",()=>{
  const f=fixture();
  const before=f.reader.view();
  assert.equal(before.condition,"UNKNOWN");
  begin(f);
  assert.equal(f.reader.view().condition,"CURRENT_PUBLISHED_CONTROL_ONLY");
  assert.equal(f.reader.view().automated_replay,false);
});
test("path changed or removed invalidates target even when WebSocket stays connected",()=>{
  for(const cmd of ["PATH_CHANGED","PATH_REMOVED","PATH_ADDED"]) {
    const f=fixture(); begin(f);
    f.reader.observeNotification(f.scope,2,f.now(),{COMMAND:cmd,DATA:"/StageCore_Lab"});
    assert.equal(f.reader.view().condition,"UNKNOWN");
    assert.equal(f.reader.view().reason,"OSCQUERY_PATH_CHANGED");
    assert.equal(f.reader.view().published_value,null);
    // Only a NEW snapshot can restore a software-only view.
    f.reader.observeSnapshot(f.scope,3,f.now(),f.snapshot(0));
    assert.equal(f.reader.view().published_value,0);
    assert.equal(f.reader.view().application_receipt_confirmed,false);
  }
});
test("rename of old or new target path invalidates, unrelated path does not",()=>{
  const f=fixture();begin(f);
  f.reader.observeNotification(f.scope,2,f.now(),{COMMAND:"PATH_CHANGED",DATA:"/Other"});
  assert.equal(f.reader.view().published_value,0.5);
  f.reader.observeNotification(f.scope,3,f.now(),{COMMAND:"PATH_RENAMED",
    DATA:{OLD:"/Old",NEW:"/StageCore_Lab/layer_opacity"}});
  assert.equal(f.reader.view().published_value,null);
  f.reader.observeSnapshot(f.scope,4,f.now(),f.snapshot(0.75));
  f.reader.observeNotification(f.scope,5,f.now(),{COMMAND:"PATH_RENAMED",
    DATA:{OLD:target,NEW:"/New"}});
  assert.equal(f.reader.view().reason,"OSCQUERY_PATH_CHANGED");
});
test("missing frame, stale/future timestamps and sequence replay never refresh evidence",()=>{
  const f=fixture();begin(f);
  const foreign={...f.scope,project_id:"project-b"};
  assert.throws(()=>f.reader.observeSnapshot(foreign,2,f.now(),f.snapshot(1)),/out-of-scope/);
  assert.throws(()=>f.reader.observeSnapshot(f.scope,1,f.now(),f.snapshot(1)),/replayed/);
  assert.throws(()=>f.reader.observeSnapshot(f.scope,2,f.now()+1,f.snapshot(1)),/stale/);
  assert.equal(f.reader.view().published_value,0.5);
  f.advance(5001);
  assert.equal(f.reader.view().published_value,null);
  assert.equal(f.reader.view().reason,"SNAPSHOT_EXPIRED");
  assert.throws(()=>f.reader.observeSnapshot(f.scope,2,f.now()-5001,f.snapshot(1)),/stale/);
  f.reader.observeSnapshot(f.scope,3,f.now(),f.snapshot(0.2));
  assert.equal(f.reader.view().published_value,0.2);
  assert.equal(f.reader.view().physical_video_output_verified,false);
});
test("disconnect and old WebSocket callback never revives old generation",()=>{
  const f=fixture();begin(f);
  assert.equal(f.reader.disconnect({...f.scope,generation:41}),false);
  assert.equal(f.reader.view().condition,"CURRENT_PUBLISHED_CONTROL_ONLY");
  assert.equal(f.reader.disconnect(f.scope),true);
  assert.equal(f.reader.view().published_value,null);
  assert.throws(()=>f.reader.observeSnapshot(f.scope,2,f.now(),f.snapshot(1)),/out-of-scope/);
  assert.throws(()=>f.reader.bindQualified(f.scope),/old OSCQuery/);
  const next={...f.scope,generation:43};
  f.reader.bindQualified(next);
  assert.equal(f.reader.view().published_value,null);
  assert.throws(()=>f.reader.observeNotification(f.scope,100,f.now(),
    {COMMAND:"PATH_CHANGED",DATA:target}),/out-of-scope/);
  f.reader.observeSnapshot(next,1,f.now(),f.snapshot(0.8));
  assert.equal(f.reader.view().published_value,0.8);
});
test("new Project/Role/Snapshot cannot be smuggled in via a higher generation",()=>{
  const f=fixture();begin(f);
  const combinations=[
    {...f.scope,generation:43,project_id:"project-b"},
    {...f.scope,generation:43,runtime_snapshot_id:"snapshot-other"},
    {...f.scope,generation:43,machine_role_id:"OTHER"},
    {...f.scope,generation:43,receiver_instance_id:"untrusted-new-instance"},
  ];
  for(const candidate of combinations){
    assert.throws(()=>f.reader.bindQualified(candidate),/untrusted/);
  }
  assert.equal(f.reader.view().published_value,0.5);
});
test("clock reversal forces a new qualified generation",()=>{
  const f=fixture();begin(f);
  f.advance(-10);
  assert.throws(()=>f.reader.view(),/clock uncertain/);
  f.advance(10);
  assert.equal(f.reader.view().condition,"UNKNOWN");
  assert.throws(()=>f.reader.bindQualified(f.scope),/old OSCQuery/);
  f.reader.bindQualified({...f.scope,generation:43});
  assert.equal(f.reader.view().condition,"UNKNOWN");
});
test("invalid OSCQuery structure and invalid notification fail closed",()=>{
  const f=fixture();begin(f);
  const corrupt=f.snapshot(1);
  corrupt.CONTENTS.StageCore_Lab.CONTENTS.layer_opacity.FULL_PATH="/unrelated";
  assert.throws(()=>f.reader.observeSnapshot(f.scope,2,f.now(),corrupt),/unpublished/);
  assert.equal(f.reader.view().published_value,null);
  f.reader.observeSnapshot(f.scope,3,f.now(),f.snapshot(0.5));
  assert.throws(()=>f.reader.observeNotification(f.scope,4,f.now(),
    {COMMAND:"VALUE_CHANGED",DATA:target}),/notification/);
  assert.equal(f.reader.view().published_value,null);
});
test("malformed notification path never updates or authenticates output",()=>{
  const f=fixture();begin(f);
  assert.throws(()=>f.reader.observeNotification(f.scope,2,f.now(),
    {COMMAND:"PATH_RENAMED",DATA:{OLD:"/bad//path",NEW:target}}),/path/);
  const view=f.reader.view();
  assert.equal(view.condition,"UNKNOWN");
  assert.equal(view.application_receipt_confirmed,false);
  assert.equal(view.commands_enabled,false);
});
