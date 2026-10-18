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
