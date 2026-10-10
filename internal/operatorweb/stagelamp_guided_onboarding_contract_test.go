package operatorweb

import (
    "strings"
    "testing"
)

func TestStageLampGuidedDraftOnboardingNeverPublishesOrActuates(t *testing.T) {
    js := string(mustReadOperatorContractFile(t,"static/phase4.js"))
    begin := strings.Index(js,"body.querySelectorAll(\"[data-prepare-stagelamp]\")")
    end := strings.Index(js,"body.querySelectorAll(\"[data-restore-lighting]\")",begin)
    if begin<0 || end<=begin { t.Fatal("StageLamp Draft-only setup handler missing") }
    section:=js[begin:end]
    for _,must:=range []string{
        "window.confirm(t(\"stageLampPrepareConfirm\"))",
        "/configuration",
        "/configuration/draft",
        "config.revision?.status !== \"DRAFT\"",
        "String(target.configuration?.device_id || \"\").trim() === deviceID",
        "logical_type: \"stage_device\"",
        "configuration: { device_id: deviceID }",
        "await navigate(\"cues\")",
    } {
        if !strings.Contains(section,must) { t.Errorf("missing onboarding invariant: %s",must) }
    }
    for _,forbidden:=range []string{"/publish","/runtime/start","runtime/jump","/commands","/lighting-activation","/stagelaser-assignment"} {
        if strings.Contains(section,forbidden) {t.Errorf("Draft onboarding must not perform %s",forbidden)}
    }
}

func TestLightingRecoveryOnlyCallsAuditedSnapshotSync(t *testing.T) {
    js := string(mustReadOperatorContractFile(t,"static/phase4.js"))
    begin := strings.Index(js,"body.querySelectorAll(\"[data-restore-lighting]\")")
    end := strings.Index(js,"body.querySelectorAll(\"[data-assign-stagelaser]\")",begin)
    if begin<0 || end<=begin {t.Fatal("Lighting recovery handler missing")}
    section:=js[begin:end]
    for _,must:=range []string{
        "window.confirm(t(\"lightingSyncConfirm\"))",
        "/lighting-controller",
        "config.nodes || []",
        "/stage-devices/sync-runtime-snapshot",
        "runtime_snapshot_id: assignmentSnapshotID",
        "match.status === \"SYNCED\" || match.status === \"ALREADY_SYNCED\"",
        "match.detail",
    } {
        if !strings.Contains(section,must) {t.Errorf("Lighting recovery missing safeguard %s",must)}
    }
    for _,forbidden:=range []string{"/lighting-activation","/commands","/runtime/start","/publish","gpio"} {
        if strings.Contains(section,forbidden) {t.Errorf("Lighting recovery must not directly call %s",forbidden)}
    }
}
