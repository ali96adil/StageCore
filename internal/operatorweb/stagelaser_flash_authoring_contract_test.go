package operatorweb

import (
	"strings"
	"testing"
)

func TestStageLaserFlashCueBuilderUsesSecondsAndBoundedLocalCommand(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/guided-ux.js"))
	for _, marker := range []string{
		`["LASER_SET_ON", "Laser ON"]`,
		`["LASER_SET_OFF", "Laser OFF"]`,
		`["LASER_FLASH_START", "Flash Start"]`,
		`["LASER_FLASH_STOP", "Flash Stop"]`,
		`Flash duration (seconds)`,
		`min="0.1" max="60"`,
		`Math.round(seconds * 1000)`,
		`request.duration_ms = duration`,
		`request.frequency_hz = frequency`,
		`Flash timing runs locally on StageLaser.`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("StageLaser flash authoring contract missing %q", marker)
		}
	}
	if strings.Contains(js, `StageLaser flash duration must be 1–60000 ms.`) {
		t.Fatal("Cue Builder should validate the operator's seconds input")
	}
}
