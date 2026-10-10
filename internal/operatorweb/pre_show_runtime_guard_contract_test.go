package operatorweb

import (
    "os"
    "strings"
    "testing"
)

func TestPreShowRuntimeOperatorGuardContract(t *testing.T) {
    source, err := os.ReadFile("static/app.js")
    if err != nil { t.Fatal(err) }
    text := string(source)
    required := []string{
        "renderGeneration !== state.runtimeRenderGeneration",
        "state.page !== \"runtime\"",
        "state.project?.project_id !== selectedProject",
        "preflightError",
        "const showBlocked = !preflight",
        "runtimeConnectionState",
        "state.runtimeJumpSelection",
        "recent_output_failures",
        "stagecore_uncertain_go",
        "setUncertainGO(command)",
        "request_id: command.requestID",
        "runtimeGoFlight",
        "clearUncertainGOButton",
    }
    for _, marker := range required {
        if !strings.Contains(text, marker) { t.Errorf("operator safety guard missing: %q", marker) }
    }
}
