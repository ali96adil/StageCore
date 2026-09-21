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
}

func TestStageDevicesOperatorV2CommissioningViewNeverEnablesOldControls(t *testing.T) {
	phase4 := string(mustReadOperatorContractFile(t, "static/phase4.js"))
	for _, marker := range []string{
		`device.protocol_version === "stagecore.device/2" || !canRuntime()`,
		`/api/v1/stage-devices/unassigned`,
		`/assignment/transfer-status`,
		`software_zero_report_current_connection`,
		`v2HardwareUnverified`,
		`v2NoControls`,
		`device.device_kind === "STAGE_DISPLAY" && device.protocol_version !== "stagecore.device/2"`,
		`device.device_kind === "RENDER_NODE" && device.protocol_version !== "stagecore.device/2"`,
	} {
		if !strings.Contains(phase4, marker) {
			t.Fatalf("v2 read-only safety UI missing contract marker %q", marker)
		}
	}
	if strings.Contains(phase4, `/assignment/software-transfer`) {
		t.Fatal("experimental transfer control must not be visible in Operator UI before physical qualification")
	}
}
