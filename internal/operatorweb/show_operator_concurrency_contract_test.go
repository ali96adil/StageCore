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
        "state.runtimeGoCooldownUntil = Date.now() + 1500;",
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

func TestCueWorkspacePhysicalRunsQueueAsynchronously(t *testing.T) {
    app := string(mustReadOperatorContractFile(t, "static/app.js"))
    cases := []struct{ begin, end string }{
        {"async function quickRunCueFromWorkspace(", "async function endCueCheckRehearsal("},
        {"async function testCueFromWorkspace(", "async function stopCueFromWorkspace("},
    }
    for _, tc := range cases {
        start := strings.Index(app, tc.begin)
        if start < 0 { t.Fatalf("missing Cue action %s", tc.begin) }
        after := app[start:]
        stop := strings.Index(after, tc.end)
        if stop < 0 { t.Fatalf("missing Cue action boundary %s", tc.end) }
        block := after[:stop]
        for _, marker := range []string{
            "async: true",
            "state.cueRunInFlight = true;",
            "state.cueRunInFlight = false;",
            "state.cueRunCooldownUntil = Date.now() + 1500;",
            "rememberRuntimeUncertainCommand({ projectID, action: \"JUMP\" });",
            "confirmPriorUncertainCueCommand(runtime, projectID)",
        } {
            if !strings.Contains(block, marker) {
                t.Errorf("%s must queue Cue safely: %q", tc.begin, marker)
            }
        }
    }
}
