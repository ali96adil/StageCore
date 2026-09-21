package operatorweb

import (
	"regexp"
	"strings"
	"testing"
)

func TestWorkspaceProfilePreservesPhase4InjectedNavigation(t *testing.T) {
	css := string(mustReadOperatorContractFile(t, "static/workspace-profile.css"))
	phase4 := string(mustReadOperatorContractFile(t, "static/phase4.js"))

	for _, marker := range []string{
		`["devices", t("devices")]`,
		`["callboard", t("callboard")]`,
		`["video", t("video")]`,
		`["network", t("network")]`,
		`button.dataset.phase4Nav = "true"`,
	} {
		if !strings.Contains(phase4, marker) {
			t.Fatalf("Phase 4 navigation missing contract marker %q", marker)
		}
	}

	// Keep the Phase 4 marker and visible declaration inside the same scoped
	// exemption without requiring an obsolete single-selector spelling.
	exemption := regexp.MustCompile(`(?s)#workspaceNav\s+:is\(([^)]*)\)\.f017-profile-hidden\s*\{\s*display:\s*block\s*!important;`).FindStringSubmatch(css)
	if len(exemption) != 2 || !strings.Contains(exemption[1], `[data-phase4-nav="true"]`) {
		t.Fatal("workspace profile Phase 4 visibility exemption missing or incorrectly scoped")
	}
}
