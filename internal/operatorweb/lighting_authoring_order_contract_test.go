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
		"let allCueModel = { cues: [] };",
		"[controllerModel, cueModel, allCueModel] = await Promise.all([",
		"api(`/api/v1/projects/${encodeURIComponent(pid())}/cues`)",
		"return (allCueModel.cues || []).reduce(",
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("lighting cue ordering missing contract marker %q", marker)
		}
	}
	if strings.Contains(js, "return (cueModel.cues || []).reduce(") {
		t.Fatal("lighting cue order cannot be allocated from the filtered lighting-only list")
	}
}
