"use strict";
const test=require("node:test");
const assert=require("node:assert/strict");
const {spawnSync}=require("node:child_process");
const path=require("node:path");
const cli=path.join(__dirname,"vdmx_readback_fixture_cli.js");
const fixture=JSON.stringify({FULL_PATH:"/",CONTENTS:{
  "StageCore_Lab":{FULL_PATH:"/StageCore_Lab",CONTENTS:{
    "layer_opacity":{FULL_PATH:"/StageCore_Lab/layer_opacity",
      ACCESS:1,TYPE:"f",VALUE:[0.7]},
  }},
}});
test("offline VDMX JSON fixture CLI never claims authenticated/confirmed playback",()=>{
  const run=spawnSync(process.execPath,[cli,"/StageCore_Lab/layer_opacity"],{
    input:fixture,encoding:"utf8",timeout:5000});
  assert.equal(run.status,0,run.stderr);
  const view=JSON.parse(run.stdout);
  assert.equal(view.value,0.7);
  assert.equal(view.evidence,"PUBLISHED_CONTROL_VALUE_ONLY");
  for(const flag of ["receiver_authenticated","snapshot_current",
    "application_cue_acknowledged","video_output_verified","commands_enabled"]){
    assert.equal(view[flag],false);
  }
});
test("offline VDMX CLI refuses wrong control path without leaking fixture",()=>{
  const run=spawnSync(process.execPath,[cli,"/StageCore_Lab/secret"],{
    input:fixture,encoding:"utf8",timeout:5000});
  assert.equal(run.status,1);
  assert.equal(run.stdout,"");
  assert.match(run.stderr,/UNKNOWN/);
  assert.equal(run.stderr.includes("0.7"),false);
});
test("offline VDMX CLI bounds JSON and never obtains live data",()=>{
  const run=spawnSync(process.execPath,[cli,"/StageCore_Lab/layer_opacity"],{
    input:" ".repeat(65537),encoding:"utf8",timeout:5000});
  assert.equal(run.status,1);
  assert.equal(run.stdout,"");
  const fs=require("node:fs");
  const source=fs.readFileSync(cli,"utf8");
  for(const risky of ["fetch(", "WebSocket", "createConnection(",
    "dgram.createSocket", "udp.send(", "OSCExecutor"]){
    assert.equal(source.includes(risky),false,"unexpected active communication: "+risky);
  }
});
