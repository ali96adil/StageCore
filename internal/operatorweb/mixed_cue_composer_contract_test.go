package operatorweb

import (
	"strings"
	"testing"
)

func TestGuidedCueComposerImportsIndependentActionCopies(t *testing.T) {
	source := string(mustReadOperatorContractFile(t, "static/guided-ux.js"))

	for _, marker := range []string{
		"Compose from existing Cues",
		"Import actions",
		"f002ImportCueActions",
		"cue.cue_id !== currentCueID",
		"Array.isArray(cue.actions)",
		`action_id: ""`,
		"JSON.parse(JSON.stringify(action.parameters))",
		"JSON.parse(JSON.stringify(action.timeout_policy))",
		"JSON.parse(JSON.stringify(action.error_policy))",
		"Imported ${source.actions.length} Action(s)",
		"source Cue stays unchanged",
		"Send MIDI message",
		"Tablet Player Action",
		"f002AddTabletAction",
		"/tablet-controller/cue-actions",
		"TABLET_LIVE_SHOW",
		"TABLET_BRIGHTNESS_SET",
		"TABLET_ORIENTATION_SET",
		"TABLET_VIDEO_SCALE_SET",
		"TABLET_LIVE_ROTATION_SET",
		"f002TabletBrightness",
		"f002TabletOrientation",
		"f002TabletVideoScale",
		"f002TabletLiveRotation",
		"f002TabletVisualPayload",
		"f002TabletContentMode",
		"f002TabletLiveMode",
		"f002TabletLiveFlash",
		"parsed.searchParams.set(\"flash\", \"1\")",
		"Use JSON override",
		"Lighting Action",
		"f002AddLightingAction",
		"/lighting-controller/cue-actions",
		"LIGHTING_CHANNELS_FADE",
		"f002LightingChannel",
		"f002LightingLevel",
		"StageLaser Action",
		"f002AddStageLaserAction",
		"/stagelaser-controller/cue-actions",
		"LASER_ARM",
		"LASER_DISARM",
		"LASER_SET_ON",
		"LASER_SET_OFF",
		"LASER_FLASH_START",
		"LASER_FLASH_STOP",
		"LASER_SAFE_OFF",
		"f002StageLaserDevice",
		"f002StageLaserCommand",
		"f002StageLaserFrequency",
		"f002StageLaserDuration",
		"never a raw toggle",
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("mixed Cue composer missing contract marker %q", marker)
		}
	}

	for _, forbidden := range []string{
		"action_id: action.action_id",
		"source.actions.splice",
		"source.actions.push",
		"source.actions =",
		"LASER_TOGGLE",
		"laser.toggle",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("mixed Cue composer must not reuse IDs or mutate source Cue via %q", forbidden)
		}
	}
}
