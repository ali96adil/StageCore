package operatorweb

import (
	"strings"
	"testing"
)

func TestCueGroupEditorProvidesVisualLinkedCueComposition(t *testing.T) {
	html := string(mustReadOperatorContractFile(t, "static/index.html"))
	js := string(mustReadOperatorContractFile(t, "static/app.js"))
	css := string(mustReadOperatorContractFile(t, "static/app.css"))

	for _, marker := range []string{
		"Run together with",
		"id=\"linkedCuesEditor\"",
		"id=\"linkedCuesCount\"",
		"Selected Cues become visually linked children of this Cue",
		"Advanced execution policy",
	} {
		if !strings.Contains(html, marker) {
			t.Fatalf("Cue Group editor missing HTML marker %q", marker)
		}
	}
	for _, marker := range []string{
		"function cueLinkedIDs",
		"function cueParentMapFor",
		"function cueParentMap",
		"function cueRelationshipClass",
		"function cueRelationshipLabel",
		"function renderCueRelationship",
		"function updateLinkedCuesEditorState",
		"function renderLinkedCuesEditor",
		"function linkedCuePolicyFromEditor",
		"linked_cue_ids",
		"linked_cue_mode = \"TOGETHER\"",
		"GROUP ·",
		"LINKED CHILD",
		"<th>Group</th>",
		"runtime-cue-links",
		"dashboard-cue-links",
		"dashboardCueParents",
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("Cue Group editor missing JS marker %q", marker)
		}
	}
	for _, marker := range []string{
		".cue-row-parent",
		".cue-row-child",
		".cue-link-badge-parent",
		".cue-link-badge-child",
		".linked-cue-option.selected",
		".runtime-cue-links",
	} {
		if !strings.Contains(css, marker) {
			t.Fatalf("Cue Group visibility missing CSS marker %q", marker)
		}
	}
}
