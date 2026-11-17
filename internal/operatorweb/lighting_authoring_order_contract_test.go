package operatorweb

import (
	"strings"
	"testing"
)

// Regression: Lighting Cue Builder historically allocated order_index only
// from the filtered Lighting Cue list. An existing OSC Cue at order 1 could
// then collide with the second new Lighting Cue and surface HTTP 503.
func TestLightingCueBuilderOrdersAcrossAllRevisionCues(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/lighting-authoring.js"))
	for _, marker := range []string{
		"async function loadModels(projectID = pid())",
		"api(`/api/v1/projects/${encodeURIComponent(projectID)}/cues`)",
		"const [nextControllerModel, nextCueModel, nextAllCueModel] = await loadModels(projectID);",
		"return (allCueModel.cues || []).reduce(",
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("lighting cue ordering missing contract marker %q", marker)
		}
	}
	if strings.Contains(js, "return (cueModel.cues || []).reduce(") {
		t.Fatal("lighting cue order cannot be allocated from the filtered lighting-only list")
	}
	renderStart := strings.Index(js, "async function renderLightingCues(")
	if renderStart < 0 {
		t.Fatal("lighting Cue renderer is missing")
	}
	renderEnd := strings.Index(js[renderStart:], "\n  function bindCueList()")
	if renderEnd < 0 {
		t.Fatal("lighting Cue renderer body is not bounded")
	}
	render := js[renderStart : renderStart+renderEnd]
	projectFence := strings.Index(render, "if (pid() !== projectID) return;")
	applyModels := strings.Index(render, "controllerModel = nextControllerModel;")
	if projectFence < 0 || applyModels < 0 || projectFence > applyModels {
		t.Fatal("late lighting model responses must be fenced before they replace the selected Project view")
	}
	saveStart := strings.Index(js, "async function saveCue()")
	if saveStart < 0 {
		t.Fatal("lighting Cue save function is missing")
	}
	saveEnd := strings.Index(js[saveStart:], "\n  async function duplicateCue(")
	if saveEnd < 0 {
		t.Fatal("lighting Cue save body is not bounded")
	}
	save := js[saveStart : saveStart+saveEnd]
	translation := strings.Index(save, "lighting-controller/cue-actions")
	saveFence := strings.Index(save, "if (!editor.isConnected || pid() !== projectID) return;")
	cueWrite := strings.Index(save, "${encodeURIComponent(projectID)}/cues${cueID")
	if translation < 0 || saveFence < translation || cueWrite < 0 || saveFence > cueWrite {
		t.Fatal("lighting Cue saves must retain and recheck the source Project ID before writing")
	}
}
