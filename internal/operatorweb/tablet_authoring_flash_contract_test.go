package operatorweb

import (
	"strings"
	"testing"
)

func TestTabletAuthoringPreservesSelectiveLiveFlashIntent(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/tablet-authoring.js"))
	for _, marker := range []string{
		`liveFlash: "Use camera flash for this Live"`,
		`class="tablet-live-flash`,
		`parsed.searchParams.get("flash")`,
		`parsed.searchParams.delete("flash")`,
		`parsed.searchParams.set("flash", "1")`,
		`params.querySelector(".tablet-live-flash input")?.checked`,
		`host.querySelector(".tablet-live-flash")?.classList.toggle("hidden", !urlMode)`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("tablet scene Live flash authoring missing contract marker %q", marker)
		}
	}
}

func TestTabletAuthoringOrdersScenesAcrossAllRevisionCues(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/tablet-authoring.js"))
	for _, marker := range []string{
		`let allCueModel = { cues: [] };`,
		`[tabletModel, sceneModel, allCueModel] = await Promise.all([`,
		"api(`/api/v1/projects/${encodeURIComponent(pid())}/cues`)",
		`return (allCueModel.cues || []).reduce(`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("tablet scene ordering missing contract marker %q", marker)
		}
	}
	if strings.Contains(js, `return (sceneModel.scenes || []).reduce(`) {
		t.Fatal("tablet scene order cannot be allocated from the filtered tablet-scene list")
	}
}


func TestTabletAuthoringSupportsCueStoredDisplaySettings(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/tablet-authoring.js"))
	for _, marker := range []string{
		"TABLET_BRIGHTNESS_SET",
		"TABLET_ORIENTATION_SET",
		"TABLET_VIDEO_SCALE_SET",
		"TABLET_LIVE_ROTATION_SET",
		"tablet-param-brightness",
		"tablet-param-orientation",
		"tablet-param-scale",
		"tablet-param-live-rotation",
		"brightness_percent",
		"orientation_mode",
		"video_scale_mode",
		"live_rotation_degrees",
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("tablet scene settings authoring missing contract marker %q", marker)
		}
	}
	if strings.Contains(js, "TABLET_SHOW_MODE_SET", "") {
		t.Fatal("Show Mode must remain outside Tablet Scene Cue authoring")
	}
}
