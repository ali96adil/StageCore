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
