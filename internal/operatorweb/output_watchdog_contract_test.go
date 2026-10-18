package operatorweb

import (
 "strings"
 "testing"
)

func TestOutputWatchPreviewIsScopedBilingualAndNeverActuates(t *testing.T){
 source,err:=Read("phase4.js")
 if err!=nil{t.Fatal(err)}
 s:=string(source)
 for _,expected:=range []string{
  `data-output-watch="`,
  `/output-watch`,
  `payload?.read_only === true`,
  `payload?.physical_status === "UNKNOWN"`,
  `view.physical_output_verified === false`,
  `view.commands_enabled === false`,
  `view.project_id === projectID`,
  `view.device_id === deviceID`,
  `renderGeneration !== stageDevicesRenderGeneration`,
  `outputWatchUnavailable:`,
  `اقرأ حالة مراقبة المخرجات`,
  `Physical outputs UNKNOWN`,
  `target.innerHTML`,
  `data-output-summary`,
  `outputWatchSummary:`,
  `ملخص مراقبة مخارج المشروع`,
  `view.device_heartbeat_verified === false`,
  `view.transport_activity_recent === true`,
  `view.software_report_current === true`,
  `eligible.length > 40`,
  `start += 4`,
  `device.assignment?.project_id === projectID`,
  `companionWatchSummary:`,
  `مراقبة Companion وأدوار Ableton وVDMX`,
  `/companion-output-watch`,
  `role.midi_execution_confirmed === false`,
  `role.osc_delivery_confirmed === false`,
  `role.physical_output_verified === false`,
  `comp?.project_id === projectID`,
 }{
  if !strings.Contains(s,expected){t.Errorf("missing safe watchdog UI contract %s",expected)}
 }
 begin:=strings.Index(s,"body.querySelectorAll(\"[data-output-watch]\")")
 end:=strings.Index(s,"body.querySelectorAll(\"[data-open-workspace]\")")
 if begin<0||end<begin{t.Fatal("cannot isolate watchdog UI handler")}
 callback:=s[begin:end]
 for _,unsafe:=range []string{
  `method: "POST"`,`method: "PUT"`,`/commands`,`/runtime/go`,
  `issueCommand(`,`window.location`,`setInterval`,
 }{
  if strings.Contains(callback,unsafe){t.Errorf("non-read-only action in watchdog UI: %s",unsafe)}
 }
}

func TestOutputWatchProjectSummaryNeverPollsOrDispatches(t *testing.T) {
 b,err:=Read("phase4.js")
 if err!=nil {t.Fatal(err)}
 s:=string(b)
 start:=strings.Index(s,`body.querySelector("[data-output-summary]")`)
 end:=strings.Index(s,`body.querySelectorAll("[data-output-watch]")`)
 if start<0 || end<=start {t.Fatal("missing bounded Output Watch summary handler")}
 callback:=s[start:end]
 for _,allowed:=range []string{
  `Promise.all(batch.map`,`/output-watch`,`automated_replay === false`,
  `currentProjectID() !== projectID`,`physical_status === "UNKNOWN"`,
  `transport_activity_recent === true`,`software_report_current === true`,
 } {if !strings.Contains(callback,allowed){t.Errorf("missing summary guard %s",allowed)}}
 for _,disallowed:=range []string{
  `method: "POST"`,`method: "PUT"`,`method: "DELETE"`,
  `issueCommand(`,`/runtime/go`,`/runtime/stop`,`setInterval(`,
 }{if strings.Contains(callback,disallowed){t.Errorf("unsafe summary action %s",disallowed)}}
}

func TestCompanionOutputWatchOperatorSummaryNeverClaimsMIDIorOSC(t *testing.T){
 source,err:=Read("phase4.js")
 if err!=nil{t.Fatal(err)}
 s:=string(source)
 a:=strings.Index(s,`body.querySelector("[data-output-summary]")`)
 b:=strings.Index(s,`body.querySelectorAll("[data-output-watch]")`)
 if a<0 || b<=a{t.Fatal("missing Project Watch summary block")}
 block:=s[a:b]
 for _,want:=range []string{
  `/companion-output-watch`,
  `comp?.read_only === true`,
  `comp?.automated_replay === false`,
  `comp?.physical_status === "UNKNOWN"`,
  `comp?.project_id === projectID`,
  `role.midi_execution_confirmed === false`,
  `role.osc_delivery_confirmed === false`,
  `role.physical_output_verified === false`,
  `currentProjectID() !== projectID`,
  `companionWatchUnavailable`,
 }{
  if !strings.Contains(block,want){t.Errorf("Companion UI guard missing: %s",want)}
 }
 for _,bad:=range []string{`method: "POST"`,`method: "PUT"`,`/runtime/go`,`/runtime/stop`,`setInterval(`,`issueCommand(`} {
  if strings.Contains(block,bad){t.Errorf("Companion Watch contains actuation/polling: %s",bad)}
 }
}
