package operatorweb

import (
    "strings"
    "testing"
)

// A delayed Cue workspace response must never redraw the Runtime operator page.
func TestCueWorkspaceAsyncRenderingIsNavigationFenced(t *testing.T) {
    app := string(mustReadOperatorContractFile(t, "static/app.js"))
    for _, marker := range []string{
        "state.navigationGeneration += 1;",
        "const navigationGeneration = state.navigationGeneration;",
        "const cueGeneration = ++state.cueRenderGeneration;",
        `state.page !== "cues" || state.project?.project_id !== projectRef`,
        "state.navigationGeneration !== navigationGeneration",
        "state.cueRenderGeneration !== cueGeneration",
    } {
        if !strings.Contains(app, marker) {
            t.Errorf("Cue workspace navigation fence is missing: %q", marker)
        }
    }
    for _, path := range []string{"static/show-lock.js", "static/guided-ux.js"} {
        js := string(mustReadOperatorContractFile(t, path))
        for _, marker := range []string{
            "const generation = state.navigationGeneration;",
            "state.page !== \"cues\" || state.project?.project_id !== projectRef",
            "state.navigationGeneration !== generation",
        } {
            if !strings.Contains(js, marker) {
                t.Errorf("%s must ignore stale Cue-page responses: %q", path, marker)
            }
        }
    }
}

func TestOperatorGoIsDebouncedWithoutWaitingForPriorExecution(t *testing.T) {
    app := string(mustReadOperatorContractFile(t, "static/app.js"))
    for _, marker := range []string{
        "runtimeGoCooldownUntil: 0",
        "state.runtimeGoCooldownUntil = now + 1500;",
        "async: true",
        "Date.now() < state.runtimeGoCooldownUntil",
    } {
        if !strings.Contains(app, marker) {
            t.Errorf("Operator GO admission is missing: %q", marker)
        }
    }
    if !strings.Contains(app, "state.runtimeActionInFlight = false;") {
        t.Error("HTTP GO admission must release independently of Cue execution")
    }
}

func TestSnapshotSyncGuardsOperatorSessionStart(t *testing.T) {
    app := string(mustReadOperatorContractFile(t, "static/app.js"))
    for _, marker := range []string{
        "snapshotSyncInFlight: false",
        "state.snapshotSyncInFlight = true;",
        "state.snapshotSyncInFlight = false;",
        "if (state.snapshotSyncInFlight) {",
        "!snapshot || state.snapshotSyncInFlight",
    } {
        if !strings.Contains(app, marker) {
            t.Errorf("Operator snapshot device-sync admission is missing: %q", marker)
        }
    }
}
