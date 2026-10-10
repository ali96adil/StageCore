package operatorweb

import (
    "strings"
    "testing"
)

func TestStageLampStandaloneManualOffUX(t *testing.T) {
    js := string(mustReadOperatorContractFile(t, "static/phase4.js"))
    for _, marker := range []string{
        "stageLampIndependentTitle",
        "stageLampAssumedOff",
        "stageLampReportedState",
        "data-stagelamp-observe",
        "stageLampObserveOn",
        "stageLampObserveOff",
        "stageLampNoAssignmentNeeded",
        "stageLampWaitingOnHint",
        "stageLampObservedOnHint",
        "stageLampManualOff",
        "previous.visual_state === \"ON\"",
        "Date.now() - Date.parse(previous.checked_at) <= 120000",
        "stagelamp.maintenance.manual-off",
        "data-stagelamp-manual-off",
        "visual_state:visualState",
        "RECORD_VISUAL_OBSERVATION_ONLY",
        "stageLampDefaultOff",
        "stageLampObserveOnlineRequired",
    } {
        if !strings.Contains(js, marker) {
            t.Errorf("missing StageLamp standalone UX contract %q",marker)
        }
    }
    begin := strings.Index(js, "body.querySelectorAll(\"[data-stagelamp-observe]\")")
    end := strings.Index(js, "body.querySelectorAll(\"[data-stagelamp-manual-off]\")")
    if begin<0 || end<=begin {
        t.Fatal("standalone observed ON/OFF buttons must have an independent handler")
    }
    observation := js[begin:end]
    for _, forbidden := range []string{
        "LASER_SET_ON", "LASER_SET_OFF", "LASER_TOGGLE",
        "/publish", "/snapshot", "/runtime/start", "/stagelaser-assignment",
        "/stage-devices/sync-runtime-snapshot",
    } {
        if strings.Contains(observation, forbidden) {
            t.Errorf("visual observation must not perform %q",forbidden)
        }
    }
    if !strings.Contains(observation, "expected_device_boot_id:expectedBootID") ||
        !strings.Contains(observation,"window.confirm(prompt)") {
        t.Fatal("visual ON/OFF must be confirmed and boot-bound")
    }
    standalone := strings.Index(js, "const standaloneManual = isUnassigned;")
    if standalone < 0 {
        t.Fatal("standalone mode must be independent of assignment snapshot ID")
    }
    if strings.Contains(js[standalone:standalone+len("const standaloneManual = isUnassigned;")], "assignmentSnapshotID") {
        t.Fatal("snapshot must not gate standalone maintenance")
    }
}
