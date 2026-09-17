package operatorweb

import (
	"strings"
	"testing"
)

func TestF023AssistantWorkspaceHasNoRuntimeCommandSurface(t *testing.T) {
	source := string(mustReadOperatorContractFile(t, "static/assistant-workspace.js"))
	for _, required := range []string{
		`/assistant/status`,
		`/assistant/tasks`,
		`/assistant/proposals/preview`,
		`/assistant/proposals/${path}`,
		`? "apply-batch" : "apply"`,
		`apply-batch`,
		`assistant.evidence`,
		`assistant.assumptions`,
		`assistant.missing`,
		`assistant.proposed_changes`,
		`assistant.preview`,
		`assistant.apply`,
		`assistant.discard`,
		`assistant.rehearsal`,
		`assistant.rehearsal_hint`,
		`assistant.select_simulation`,
		`assistant.rehearsal_needs_simulation`,
		`assistantScopeSessions`,
		`"REHEARSAL"`,
		`"SIMULATION"`,
	} {
		if !strings.Contains(source, required) {
			t.Errorf("Assistant workspace missing %q", required)
		}
	}
	for _, forbidden := range []string{
		`/runtime/go`,
		`/runtime/blackout`,
		`/runtime/emergency`,
		`/panic`,
	} {
		if strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Errorf("Assistant workspace must not expose runtime command path %q", forbidden)
		}
	}
}
