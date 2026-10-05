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
		"These conditions do not stop REHEARSAL/SHOW or GO.",
		"Preflight is advisory at live Session start.",
		"unavailable Actions are recorded and skipped unless an Action explicitly uses FAIL_CUE",
		`id="startShowButton" class="button warn" ${!canControl || !snapshot ? "disabled" : ""}`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("Operator fail-soft Runtime UX missing %q", marker)
		}
	}
	if strings.Contains(js, `!canControl || !snapshot || showBlocked`) {
		t.Fatal("SHOW button still hard-blocks on Preflight status")
	}
}
