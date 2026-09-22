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
		"view?.schema_version === 1",
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

func TestStageDevicesRejectsStaleProjectInventoryAndCommands(t *testing.T) {
	source := string(mustReadOperatorContractFile(t, "static/phase4.js"))
	start := strings.Index(source, "async function renderStageDevices()")
	if start < 0 {
		t.Fatal("Stage Devices renderer missing")
	}
	end := strings.Index(source[start:], "function displayTargets(")
	if end < 0 {
		t.Fatal("Stage Devices renderer boundary missing")
	}
	view := source[start : start+end]
	if !strings.Contains(source, "let stageDevicesRenderGeneration = 0;") ||
		!strings.Contains(view, "const renderGeneration = ++stageDevicesRenderGeneration;") {
		t.Fatal("each Stage Devices render must have a unique generation")
	}
	guard := `if (renderGeneration !== stageDevicesRenderGeneration || state.page !== "devices" || currentProjectID() !== projectID) return;`
	if strings.Count(view, guard) != 2 {
		t.Fatal("device list and additional inventory must each reject stale project/page responses")
	}
	paint := strings.Index(view, "body.innerHTML =")
	if paint < 0 || strings.LastIndex(view[:paint], guard) < 0 {
		t.Fatal("Stage Devices must recheck the project immediately before painting")
	}
	if !strings.Contains(view, `if (!button.isConnected || renderGeneration !== stageDevicesRenderGeneration || state.page !== "devices" || currentProjectID() !== projectID) return;`) {
		t.Fatal("old Stage Devices controls must never dispatch a command")
	}
	if !strings.Contains(view, `if (renderGeneration === stageDevicesRenderGeneration && state.page === "devices" && currentProjectID() === projectID) await renderStageDevices();`) {
		t.Fatal("command completion must not repaint another Project")
	}
	if !strings.Contains(view, `target.isConnected && renderGeneration === stageDevicesRenderGeneration && state.page === "devices" && currentProjectID() === projectID`) {
		t.Fatal("failed diagnostic response must not paint stale Project context")
	}
}
