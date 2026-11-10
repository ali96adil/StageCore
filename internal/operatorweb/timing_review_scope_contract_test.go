package operatorweb

import (
	"strings"
	"testing"
)

func TestTimingReviewWithholdsStaleProjectAndSessionDisplay(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/timing-intelligence.js"))
	for _, marker := range []string{
		`const reviewUser = state.user;`,
		`const reviewProjectID = state.project.project_id;`,
		`state.user === reviewUser`,
		`state.project?.project_id === reviewProjectID && state.page === "timing"`,
		`if (!isCurrentReview()) return;`,
		`Backend ACL protects the response; this prevents stale browser display.`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("Timing report lost its project/user scope guard: %s", marker)
		}
	}
	if strings.Count(js, `if (!isCurrentReview()) return;`) != 2 {
		t.Fatal("Timing must guard both failed fetch display and successful report display")
	}
}
