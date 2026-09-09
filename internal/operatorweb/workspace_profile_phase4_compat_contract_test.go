package operatorweb

import (
	"strings"
	"testing"
)

func TestWorkspaceProfilePreservesPhase4InjectedNavigation(t *testing.T) {
	workspace := string(mustReadOperatorContractFile(t, "static/workspace-profile.js"))
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

	for _, marker := range []string{
		`if (!F017_PAGES.includes(page)) {`,
		`button.classList.remove("f017-profile-hidden")`,
		`continue;`,
	} {
		if !strings.Contains(workspace, marker) {
			t.Fatalf("workspace profile compatibility guard missing %q", marker)
		}
	}
}
