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
		`function sessionComparisonMetrics(session, cues, events)`,
		`function sessionComparisonMarkup(primarySession, primaryCues, primaryEvents, otherSession, otherCues, otherEvents)`,
		`async function prepareSessionComparison(primarySession, primaryCues, primaryEvents, projectID, current)`,
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
