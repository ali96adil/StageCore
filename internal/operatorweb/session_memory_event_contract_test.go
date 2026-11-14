package operatorweb

import (
	"strings"
	"testing"
)

func TestSessionMemoryTimelineUsesWhitelistedEventsAndSafeExport(t *testing.T) {
	asset := string(mustReadOperatorContractFile(t, "static/memory.js"))
	for _, marker := range []string{
		`detail.events || []`,
		`function sessionEventTimelineMarkup(events, category = "all")`,
		`function sessionTraceExportProjection(session, cues, events)`,
		`function downloadSessionTrace(session, cues, events)`,
		`event.category === category`,
		`action.delivery_evidence`,
		`id="sessionEventCategory"`,
		`id="exportSessionTrace"`,
	} {
		if !strings.Contains(asset, marker) {
			t.Errorf("Session Memory timeline is missing %q", marker)
		}
	}
	if strings.Contains(asset, `response_summary: action.response_summary`) {
		t.Fatal("Session Memory export must omit free-form adapter response text")
	}
}
