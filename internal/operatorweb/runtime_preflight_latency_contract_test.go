package operatorweb

import (
    "strings"
    "testing"
)

func TestRuntimePreflightDoesNotBlockCueDisplayOrGo(t *testing.T) {
    js := string(mustReadOperatorContractFile(t, "static/app.js"))
    if strings.Contains(js, "const [runtime, preflight] = await Promise.all([") {
        t.Fatal("Runtime still waits for costly Preflight on every render")
    }
    for _, marker := range []string{
        "function runtimePreflightSnapshot(projectID)",
        "state.runtimePreflightPending",
        "state.runtimePreflightEpoch !== epoch",
        "const snapshotID = runtime.runtime_snapshot?.runtime_snapshot_id || null;",
        "now - state.runtimePreflightCheckedAt >= 10000",
        "const cachedPreflight = runtimePreflightSnapshot(activeProjectID);",
        "const runtime = await api(",
        "const showBlocked = !preflight || ![\"PASS\", \"WARN\"].includes(preflight.status);",
        "preflight ? \"READY\" : \"UNKNOWN\"",
        "const renderSequence = ++state.runtimeRenderSequence",
        "renderSequence !== state.runtimeRenderSequence",
        "if (state.runtimeGoPending || state.runtimeJumpPending) return;",
        "function loadRuntimeUncertainCommand()",
        "function rememberRuntimeUncertainCommand(command)",
        "function confirmPriorUncertainCueCommand(runtime, projectID)",
        "runtimeUncertainCommand: loadRuntimeUncertainCommand()",
        "state.runtimeJumpPending",
        "Previous ${esc(state.runtimeUncertainCommand.action)} response unknown",
        "function patchActiveRuntimeView(markup)",
        "state.runtimeViewKey === viewKey",
        "patchActiveRuntimeView(runtimeMarkup)",
        "expected_current_cue_id: runtime.current_cue?.cue_id || null, async: true",
    } {
        if !strings.Contains(js, marker) {
            t.Errorf("Runtime latency/safety invariant missing %q", marker)
        }
    }
}

func TestPreflightNavigationPaintsBeforeFullScan(t *testing.T) {
    js := string(mustReadOperatorContractFile(t, "static/preflight.js"))
    start := strings.Index(js, "navigate = async function stagecoreNavigate(page)")
    if start < 0 {
        t.Fatal("Preflight navigation hook missing")
    }
    navigation := js[start:]
    paint := strings.Index(navigation, "content.innerHTML =")
    scan := strings.Index(navigation, "try { await renderPreflight(); }")
    if paint < 0 || scan < 0 || paint >= scan {
        t.Fatal("Preflight navigation does not paint loading state before the live scan")
    }
    for _, marker := range []string{
        "state.page !== \"preflight\" || state.project?.project_id !== projectID",
        "recordRuntimePreflight(projectID, report)",
        "sequence !== state.preflightRenderSequence",
        "Readiness unavailable",
    } {
        if !strings.Contains(js, marker) {
            t.Errorf("Preflight latency/safety invariant missing %q", marker)
        }
    }
}
