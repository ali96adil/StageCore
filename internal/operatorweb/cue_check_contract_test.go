package operatorweb

import (
	"strings"
	"testing"
)

func TestCueWorkspaceProvidesPublishedCueCheckControls(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/app.js"))

	for _, marker := range []string{
		"Run Published Cue in Rehearsal",
		"Run Cue in Rehearsal",
		"STOP CUE",
		"BLACKOUT",
		"CLEAR BLACKOUT",
		"async function testCueFromWorkspace",
		"runtime.mode !== \"REHEARSAL\"",
		"/runtime/jump",
		"runtime.mode === \"SHOW\"",
		"Publish before testing it.",
		"async function setCueWorkspaceBlackout",
		"emergency-blackout",
		"project-blackout",
		"Create Draft to edit",
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("Cue Workspace check controls missing contract marker %q", marker)
		}
	}

	if strings.Contains(js, "Test Draft Cue") {
		t.Fatal("Cue Check must not imply that unpublished Draft behavior is executed")
	}
	start := strings.Index(js, "async function testCueFromWorkspace(cueID)")
	end := -1
	if start >= 0 {
		relativeEnd := strings.Index(js[start:], "async function stopCueFromWorkspace()")
		if relativeEnd >= 0 {
			end = start + relativeEnd
		}
	}
	if start < 0 || end < 0 || start >= end {
		t.Fatal("cannot identify Cue Workspace execution handler")
	}
	handler := js[start:end]
	for _, forbidden := range []string{
		"await api(`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/runtime/start`",
		"startRehearsal",
		"mode: \"REHEARSAL\"",
	} {
		if strings.Contains(handler, forbidden) {
			t.Fatalf("Cue Workspace must never create a session; found %q", forbidden)
		}
	}
	for _, required := range []string{
		"runtime.mode !== \"REHEARSAL\"",
		"runtime.session?.type !== \"REHEARSAL\"",
		"runtime/jump",
		"ALL actions of published Cue",
		"CURRENT REHEARSAL",
	} {
		if !strings.Contains(handler, required) {
			t.Errorf("Cue Workspace missing explicit-rehearsal invariant %q", required)
		}
	}
}
