package operatorweb

import (
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

	for _, marker := range []string{
		`#workspaceNav [data-phase4-nav="true"].f017-profile-hidden`,
		`display: block !important;`,
	} {
		if !strings.Contains(css, marker) {
			t.Fatalf("workspace profile Phase 4 compatibility rule missing %q", marker)
		}
	}
}
