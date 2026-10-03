package operatorweb

import (
	"strings"
	"testing"
)

func TestCueWorkspaceProvidesPublishedCueCheckControls(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/app.js"))

	for _, marker := range []string{
		"Cue Check",
		"Test Cue",
		"STOP CUE",
		"BLACKOUT",
		"CLEAR BLACKOUT",
		"async function testCueFromWorkspace",
		"mode: \"REHEARSAL\"",
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
}
