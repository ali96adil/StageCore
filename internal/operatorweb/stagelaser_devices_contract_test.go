package operatorweb

import (
	"strings"
	"testing"
)

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
		`حالة الليزر`,
		`لا تفترض أنه مطفأ`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("StageLaser Devices surface missing contract marker %q", marker)
		}
	}
	if strings.Contains(js, "LASER_TOGGLE") || strings.Contains(js, "laser.toggle") {
		t.Fatal("StageLaser Devices surface must never expose raw toggle semantics")
	}
}
