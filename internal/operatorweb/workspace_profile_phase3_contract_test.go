package operatorweb

import (
	"strings"
	"testing"
)

func TestWorkspaceProfilePhase3PagesContract(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/workspace-profile-phase3.js"))

	for _, marker := range []string{
		`const phase3Pages = ["timecode", "timing"]`,
		`F017_PAGES.push(page)`,
		`workspace.page.timecode`,
		`workspace.page.timing`,
		`F017_PRESETS["stage-manager"]`,
		`stageManager.visible_pages = insertAfterRuntime(stageManager.visible_pages)`,
		`stageManager.page_order = insertAfterRuntime(stageManager.page_order)`,
		`f017ApplyProfile({ navigateIfNeeded: false })`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("Phase 3 workspace integration missing contract marker %q", marker)
		}
	}

	for _, forbidden := range []string{
		`api(`,
		`fetch(`,
		`method: "POST"`,
		`method: "PUT"`,
		`method: "PATCH"`,
		`method: "DELETE"`,
		`/runtime/`,
		`/publish`,
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("Phase 3 workspace integration must remain presentation-only; found %q", forbidden)
		}
	}
}
