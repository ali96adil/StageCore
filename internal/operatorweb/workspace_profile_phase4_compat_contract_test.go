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
		`button.dataset.phase4CoreNav = "true"`,
		`nav.querySelector('[data-phase4-core-nav="true"]')`,
		`document.querySelectorAll('[data-phase4-core-nav="true"]')`,
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

	for _, asset := range []string{
		"static/tablet-controller.js",
		"static/tablet-authoring.js",
		"static/lighting-configuration.js",
		"static/lighting-authoring.js",
		"static/visual-engine.js",
		"static/simulation.js",
	} {
		source := string(mustReadOperatorContractFile(t, asset))
		if !strings.Contains(source, `button.dataset.phase4Nav = "true"`) {
			t.Fatalf("%s must mark its injected workspace navigation as Phase 4 compatible", asset)
		}
	}
}
