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
		"Source Cue stays unchanged",
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
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("mixed Cue composer must not reuse IDs or mutate source Cue via %q", forbidden)
		}
	}
}
