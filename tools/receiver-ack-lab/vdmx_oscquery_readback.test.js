"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const { readVDMXPublishedControl } = require("./vdmx_oscquery_readback");

function oscQueryFixture(value = 0.75) {
  // Mimics OSCQueryProposal FULL_PATH/CONTENTS/VALUE, not a live VDMX API.
  return {
    FULL_PATH: "/", ACCESS: 0,
    CONTENTS: {
      "StageCore_Lab": {
        FULL_PATH: "/StageCore_Lab", ACCESS: 0,
        CONTENTS: {
          "layer_opacity": {
            FULL_PATH: "/StageCore_Lab/layer_opacity",
            TYPE: "f", ACCESS: 3, VALUE: [value],
          },
        },
      },
    },
  };
}
const target="/StageCore_Lab/layer_opacity";

test("VDMX OSCQuery published Control Surface reports software value only", () => {
  const result=readVDMXPublishedControl(oscQueryFixture(),target);
  assert.deepEqual(result,{
    property:target,value:0.75,evidence:"PUBLISHED_CONTROL_VALUE_ONLY",
    actualVideoOutputVerified:false,commandsEnabled:false,
    authenticatedReceiver:false,
  });
});
test("read-only and read-write published values are both readable", () => {
  const data=oscQueryFixture(0);
  const leaf=data.CONTENTS.StageCore_Lab.CONTENTS.layer_opacity;
  leaf.ACCESS=1;
  assert.equal(readVDMXPublishedControl(data,target).value,0);
  leaf.ACCESS=2;
  assert.throws(()=>readVDMXPublishedControl(data,target),/readable/);
  leaf.ACCESS=undefined;
  assert.throws(()=>readVDMXPublishedControl(data,target),/readable/);
});
test("does not substitute an OSC input destination for a published VDMX state",()=>{
  const data=oscQueryFixture();
  assert.throws(()=>readVDMXPublishedControl(data,"/osc/input/layer_opacity"),/unpublished/);
  assert.throws(()=>readVDMXPublishedControl(data,"/StageCore_Lab/other"),/unpublished/);
  const leaf=data.CONTENTS.StageCore_Lab.CONTENTS.layer_opacity;
  leaf.FULL_PATH="/DifferentProject/layer_opacity";
  assert.throws(()=>readVDMXPublishedControl(data,target),/FULL_PATH/);
});
test("rejects multi-parameter and invalid OSCQuery leaf types",()=>{
  const data=oscQueryFixture();
  const leaf=data.CONTENTS.StageCore_Lab.CONTENTS.layer_opacity;
  leaf.VALUE=[0.3,0.4];leaf.TYPE="ff";
  assert.throws(()=>readVDMXPublishedControl(data,target),/readable/);
  leaf.VALUE=["0.3"];leaf.TYPE="f";
  assert.throws(()=>readVDMXPublishedControl(data,target),/floating/);
  leaf.VALUE=[Infinity];leaf.TYPE="f";
  assert.throws(()=>readVDMXPublishedControl(data,target),/floating/);
  leaf.VALUE=[true];leaf.TYPE="T";
  assert.equal(readVDMXPublishedControl(data,target).value,true);
  leaf.VALUE=[false];leaf.TYPE="F";
  assert.equal(readVDMXPublishedControl(data,target).value,false);
  leaf.VALUE=[1];leaf.TYPE="T";
  assert.throws(()=>readVDMXPublishedControl(data,target),/TRUE/);
  leaf.VALUE=["scene-a"];leaf.TYPE="s";
  assert.equal(readVDMXPublishedControl(data,target).value,"scene-a");
});
test("bounded parser rejects malformed child names, paths and input",()=>{
  const data=oscQueryFixture();
  assert.throws(()=>readVDMXPublishedControl(data,"/../secret"),/invalid/);
  assert.throws(()=>readVDMXPublishedControl(data,"/StageCore_Lab//layer_opacity"),/invalid/);
  assert.throws(()=>readVDMXPublishedControl(data,"/StageCore_Lab/layer_opacity/"),/invalid/);
  assert.throws(()=>readVDMXPublishedControl(null,target),/invalid/);
  const leaf=data.CONTENTS.StageCore_Lab.CONTENTS.layer_opacity;
  leaf.VALUE=[0.5];leaf.EXTRA="x".repeat(65536);
  assert.throws(()=>readVDMXPublishedControl(data,target),/too large/);
  delete leaf.EXTRA;
  data.CONTENTS.StageCore_Lab.FULL_PATH="/Other";
  assert.throws(()=>readVDMXPublishedControl(data,target),/FULL_PATH/);
});
test("independent OSCQuery state cannot by itself trigger signed ACK",()=>{
  const { AppObserverLab,VDMX_PROPERTY }=require("./app_observer");
  const now=1_800_000_000_000;
  const key=Buffer.alloc(32,0x31);
  const monitor=new AppObserverLab({key,now:()=>now});
  const target="/StageCore_Lab/layer_opacity";
  const view=readVDMXPublishedControl(oscQueryFixture(1),target);
  assert.equal(view.authenticatedReceiver,false);
  assert.equal(monitor.status().pending,false);
  assert.deepEqual(monitor.observe({
    source:VDMX_PROPERTY,property:view.property,value:view.value,
    change:true,observation_sequence:1,observed_at_ms:now,
  }),[]);
});
