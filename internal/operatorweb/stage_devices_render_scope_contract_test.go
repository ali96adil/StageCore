package operatorweb

import (
	"strings"
	"testing"
)

// Regression: a Stage Device card must not reach into renderStageDevices-local
// visualChecks, assignmentLocked, or canPair variables. When it did, the
// presence of any device blanked the whole operator Devices page with
// "Can't find variable: visualChecks".
func TestStageDevicesCardVisualChecksAreExplicitlyScoped(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/phase4.js"))
	for _, marker := range []string{
		"function deviceCard(device, v2Status = null, showLocked = false, visualCheck = null, canPair = false)",
		"${stageLaserVisualCheckMarkup(device, visualCheck, !showLocked && canPair)}",
		"deviceCard(device, statuses[device.device_id], assignmentLocked, visualChecks[device.device_id], canPair)",
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("Stage Devices card missing explicitly scoped visual-check contract: %q", marker)
		}
	}
	if strings.Contains(js, "${stageLaserVisualCheckMarkup(device, visualChecks[device.device_id], !assignmentLocked && canPair)}\n        ${isStageLaser(device) ?") {
		t.Fatal("Stage Device card illegally reads renderStageDevices-local state")
	}
}
