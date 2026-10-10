package operatorweb

import (
    "strings"
    "testing"
)

func TestStageLampManualOffDoesNotRequireSnapshotOrDispatchCue(t *testing.T) {
    js:=string(mustReadOperatorContractFile(t,"static/phase4.js"))
    for _,marker:=range []string{
        "stagelamp.maintenance.manual-off",
        "data-stagelamp-manual-off",
        "data-check-id",
        "previous.visual_state === \"ON\"",
        "previous.device_boot_id !== bootID",
        "Date.now() - Date.parse(previous.checked_at) <= 120000",
        "/stagelamp/manual-off",
        "PULSE_ONCE_TO_TURN_OFF_OBSERVED_ON_LAMP",
        "stageLampManualOffConfirm",
        "stageLampManualOffApplied",
        "stageLampDefaultOff",
    } {
        if !strings.Contains(js,marker) { t.Errorf("missing manual OFF marker %q",marker) }
    }
    begin:=strings.Index(js,"body.querySelectorAll(\"[data-stagelamp-manual-off]\")")
    end:=-1
    if begin>=0 {
        if offset:=strings.Index(js[begin:],"body.querySelectorAll(\"[data-stage-laser-visual-save]\")");offset>=0 {end=begin+offset}
    }
    if begin<0 || end<=begin {t.Fatal("manual OFF handler missing")}
    section:=js[begin:end]
    for _,forbidden:=range []string{
        "/publish","/runtime/start","/runtime/jump",
        "/stagelaser-assignment","/lighting-activation",
    } {
        if strings.Contains(section,forbidden) { t.Errorf("manual OFF must not call %q",forbidden) }
    }
    if !strings.Contains(section,"button.disabled = true") ||
        strings.Contains(section,"button.disabled = false") {
        t.Fatal("manual OFF button must never auto-reenable after an uncertain toggle")
    }
}
