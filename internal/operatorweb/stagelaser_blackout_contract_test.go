package operatorweb

import (
	"strings"
	"testing"
)

func TestManagedBlackoutSurfaceIncludesStageLaserFailClosedRecovery(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/app.js"))
	for _, marker := range []string{
		`["StageLaser", payload.stagelaser]`,
		"managed Lighting, StageLaser, Tablet and Native Visual outputs",
		"StageLaser stays DISARMED/OFF",
		"ARM / Laser Cue actions",
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("managed blackout StageLaser surface missing %q", marker)
		}
	}
	for _, forbidden := range []string{
		"clear StageLaser blackout automatically",
		"LASER_TOGGLE",
		"laser.toggle",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("managed blackout UI must not expose unsafe StageLaser recovery via %q", forbidden)
		}
	}
}
