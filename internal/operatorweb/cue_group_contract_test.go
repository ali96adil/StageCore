package operatorweb

import (
	"strings"
	"testing"
)

func TestCueGroupEditorProvidesVisualLinkedCueComposition(t *testing.T) {
	html := string(mustReadOperatorContractFile(t, "static/index.html"))
	js := string(mustReadOperatorContractFile(t, "static/app.js"))

	for _, marker := range []string{
		"Run together with",
		"id=\"linkedCuesEditor\"",
		"Selected Cues become children of this Cue",
		"Advanced execution policy",
	} {
		if !strings.Contains(html, marker) {
			t.Fatalf("Cue Group editor missing HTML marker %q", marker)
		}
	}
	for _, marker := range []string{
		"function cueLinkedIDs",
		"function cueParentMap",
		"function renderLinkedCuesEditor",
		"function linkedCuePolicyFromEditor",
		"linked_cue_ids",
		"linked_cue_mode = \"TOGETHER\"",
		"Child of",
		"Runs together:",
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("Cue Group editor missing JS marker %q", marker)
		}
	}
}
