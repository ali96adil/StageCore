package operatorweb

import (
	"strings"
	"testing"
)

func TestRuntimeUsesFailSoftDegradedOperationUX(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/app.js"))
	for _, marker := range []string{
		"degraded_reasons",
		"device_scope_warning",
		"preflight_warning",
		"Session started in DEGRADED mode.",
		"Runtime readiness · degraded operation allowed",
		"WARN means degraded live operation is allowed",
		"Structural Snapshot/security/storage/timecode configuration BLOCK conditions still prevent SHOW entry.",
		"unless FAIL_CUE is explicit",
		`id="startShowButton" class="button warn" ${!canControl || !snapshot || showBlocked || preflightUnavailable ? "disabled" : ""}`,
		`return ["OWNER", "TECHNICIAN", "OPERATOR"].includes(state.user?.role)`,
		"advisory for live start",
		"groupRuntimeReadinessIssues",
		"runtimeReadinessGroupTitle",
		"Repeated offline / stale Snapshot / stale network observations are grouped by affected resource below.",
		"RESOURCE",
		"DETAIL",
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("Operator fail-soft Runtime UX missing %q", marker)
		}
	}
}
