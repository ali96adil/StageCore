package operatorweb

import (
	"strings"
	"testing"
)

func TestStageDevicesReadOnlyLightingDiagnosticContract(t *testing.T) {
	source := string(mustReadOperatorContractFile(t, "static/phase4.js"))
	css := string(mustReadOperatorContractFile(t, "static/phase4.css"))
	required := []string{
		"data-live-diagnostic-device",
		"stagecore.esp32-dmx-lighting-node",
		"renderLightingDiagnostic(view)",
		"lighting-controller/nodes/",
		"/live-diagnostic",
		"diagnosticNoCommand",
		"diagnosticSource",
		"diagnosticUnknown",
		"diagnosticBlocked",
		"diagnosticUnsafe",
		"SOFTWARE_ONLY",
		"currentProjectID() !== projectID",
		"diagnosticUnavailable",
	}
	for _, needle := range required {
		if !strings.Contains(source, needle) {
			t.Errorf("Stage Devices diagnostic missing %q", needle)
		}
	}
	start := strings.LastIndex(source, "data-live-diagnostic-device")
	if start < 0 {
		t.Fatal("no read-only diagnostic event handler")
	}
	next := strings.Index(source[start:], "body.querySelectorAll(")
	if next < 0 {
		t.Fatal("diagnostic must have its own handler before generic command handlers")
	}
	handler := source[start : start+next]
	if !strings.Contains(handler, "await api(") ||
		!strings.Contains(handler, "/live-diagnostic") {
		t.Fatal("diagnostic must use the scoped GET route")
	}
	for _, forbidden := range []string{
		"issueCommand(", "method:", "command_type",
	} {
		if strings.Contains(handler, forbidden) {
			t.Errorf("read-only lighting diagnostic exposes mutating action %q", forbidden)
		}
	}
	if !strings.Contains(css, ".phase4-lighting-diagnostic-info") ||
		!strings.Contains(css, ".phase4-pulse.blocked") ||
		!strings.Contains(css, ".phase4-pulse.unsafe") {
		t.Fatal("BLOCKED and UNSAFE diagnostics need distinct presentation")
	}
}
