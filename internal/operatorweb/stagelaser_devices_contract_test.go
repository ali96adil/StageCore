package operatorweb

import (
	"strings"
	"testing"
)

// ONLINE is transport state, never proof of physical lamp state or output authority.
// The Stage Devices inventory must not reuse DMX/lighting status for a StageLaser.
func TestStageLaserOnlineMessagingIsEmissionSafe(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/phase4.js"))
	for _, marker := range []string{
		`stageLaserTransportOnly: "ONLINE confirms authenticated communication only.`,
		`stageLaserTransportOnly: "StageLaser هو اسم مصباح المسرح داخل النظام.`,
		`stageLaserReadOnly: "StageLaser monitoring only;`,
		`stageLaserReadOnly: "متابعة StageLaser فقط؛`,
		`v2GenericHardwareUnverified:`,
		`isStageLaser(device) ? `,
		`stageLaser ? `,
		`t("stageLampManualOffRequiresOn")`,
		`<p>${esc(t("stageLaserTransportOnly"))}</p>`,
		`device.profile_id === "stagecore.esp32-dmx-lighting-node" ? "v2HardwareUnverified" : "v2GenericHardwareUnverified"`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("StageLaser profile-sensitive status missing %q", marker)
		}
	}
	if strings.Count(js, "stageLaserTransportOnly:") != 2 {
		t.Fatal("StageLaser transport-safety note must be translated in English and Arabic")
	}
}

func TestStageDevicesSurfaceSupportsStageLaserSafeAssignmentAndTelemetry(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/phase4.js"))

	for _, marker := range []string{
		`device?.profile_id === "stagecore.esp32-stagelaser"`,
		`function stageLaserTelemetryMarkup(device, compact = false)`,
		`observed.arm_state`,
		`observed.logical_state`,
		`observed.state_quality`,
		`observed.wifi_rssi_dbm`,
		`observed.ip_address`,
		`observed.firmware_version`,
		`observed.uptime_seconds`,
		`observed.relay_pulse_count`,
		`observed.last_command_type`,
		`stageLaserTracked`,
		`stageLaserUnknown`,
		`data-assign-stagelaser`,
		`/stagelaser-assignment`,
		`VERIFY_SAFE_OFF_AND_ASSIGN_STAGELASER`,
		`expected_assignment_epoch: epoch`,
		`runtime_snapshot_id: assignmentSnapshotID`,
		`stageLaserAssignConfirm`,
		`stageLaserAssigned`,
		`حالة المصباح`,
		`تحتاج تصحيح OFF/ON بعد المشاهدة`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("StageLaser Devices surface missing contract marker %q", marker)
		}
	}
	if strings.Contains(js, "LASER_TOGGLE") || strings.Contains(js, "laser.toggle") {
		t.Fatal("StageLaser Devices surface must never expose raw toggle semantics")
	}
}
