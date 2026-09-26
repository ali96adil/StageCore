package operatorweb

import (
	"strings"
	"testing"
)

func TestWorkspaceProfilePreservesPhase4InjectedNavigation(t *testing.T) {
	css := string(mustReadOperatorContractFile(t, "static/workspace-profile.css"))
	phase4 := string(mustReadOperatorContractFile(t, "static/phase4.js"))

	for _, marker := range []string{
		`["devices", t("devices")]`,
		`["callboard", t("callboard")]`,
		`["video", t("video")]`,
		`["network", t("network")]`,
		`button.dataset.phase4Nav = "true"`,
		`button.dataset.phase4CoreNav = "true"`,
		`nav.querySelector('[data-phase4-core-nav="true"]')`,
		`document.querySelectorAll('[data-phase4-core-nav="true"]')`,
	} {
		if !strings.Contains(phase4, marker) {
			t.Fatalf("Phase 4 navigation missing contract marker %q", marker)
		}
	}

	for _, marker := range []string{
		`#workspaceNav [data-phase4-nav="true"].f017-profile-hidden`,
		`display: block !important;`,
	} {
		if !strings.Contains(css, marker) {
			t.Fatalf("workspace profile Phase 4 compatibility rule missing %q", marker)
		}
	}

	for _, asset := range []string{
		"static/tablet-controller.js",
		"static/tablet-authoring.js",
		"static/lighting-configuration.js",
		"static/lighting-authoring.js",
		"static/visual-engine.js",
		"static/simulation.js",
	} {
		source := string(mustReadOperatorContractFile(t, asset))
		if !strings.Contains(source, `button.dataset.phase4Nav = "true"`) {
			t.Fatalf("%s must mark its injected workspace navigation as Phase 4 compatible", asset)
		}
	}

}

func TestStageDevicesOperatorSeparatesLightingSafetyFromTabletAssignment(t *testing.T) {
	phase4 := string(mustReadOperatorContractFile(t, "static/phase4.js"))
	for _, marker := range []string{
		`/api/v1/stage-devices/unassigned`,
		`/assignment/transfer-status`,
		`software_zero_report_current_connection`,
		`device.profile_id === "stagecore.esp32-dmx-lighting-node"`,
		`device.profile_id === "stagecore.tablet-player"`,
		`assignment.assignment_state !== "ACTIVE"`,
		`data-assign-tablet`,
		`/api/v1/projects/${encodeURIComponent(projectID)}/runtime`,
		`/tablet-controller/devices/${encodeURIComponent(deviceID)}/assign`,
		`expected_project_id: ""`,
		`expected_runtime_snapshot_id: ""`,
		`runtime_snapshot_id: assignmentSnapshotID`,
		`v2TabletAssigned`,
		`device.device_kind === "STAGE_DISPLAY" && device.protocol_version !== "stagecore.device/2"`,
		`device.device_kind === "RENDER_NODE" && device.protocol_version !== "stagecore.device/2"`,
	} {
		if !strings.Contains(phase4, marker) {
			t.Fatalf("Stage Device v2 Operator contract missing marker %q", marker)
		}
	}
	if strings.Contains(phase4, `/assignment/software-transfer`) {
		t.Fatal("Lighting software-transfer control must remain hidden pending physical qualification")
	}
}
