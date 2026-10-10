package operatorweb

import (
	"strings"
	"testing"
)

func TestPreShowRuntimeOperatorIntegrityContracts(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/app.js"))
	for _, marker := range []string{
		"runtimeRenderGeneration",
		"state.runtimeRefreshPending",
		"state.runtimeActionInFlight",
		"generation !== state.runtimeRenderGeneration",
		"Preflight unavailable — readiness UNKNOWN",
		"preflightUnavailable ? \"disabled\"",
		"savedJump",
		"jumpSelect.value = savedJump",
		"Recent output failures",
		"runtime.recent_action_failures",
		"GO may have reached the Hub",
		"Jump may have reached the Hub",
		"Hub refresh FAILED",
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("missing pre-show operator invariant %q", marker)
		}
	}
	if strings.Contains(js, "api(`/api/v1/projects/${projectID}/preflight`).catch(() => null)") {
		t.Fatal("Preflight errors must never render as a healthy empty-check result")
	}
}
