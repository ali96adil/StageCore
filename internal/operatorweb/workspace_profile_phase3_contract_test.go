package operatorweb

import (
	"strings"
	"testing"
)

func TestWorkspaceProfileFeaturePagesContract(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/workspace-profile-phase3.js"))
	core := string(mustReadOperatorContractFile(t, "static/workspace-profile.js"))

	for _, marker := range []string{
		`f017RegisterFeaturePage`,
		`page: "timecode"`,
		`page: "timing"`,
		`page: "capsules"`,
		`page: "devices"`,
		`page: "tablet-controller"`,
		`page: "tablet-scenes"`,
		`page: "lighting-setup"`,
		`page: "lighting-cues"`,
		`page: "video"`,
		`page: "visual-engine"`,
		`page: "callboard"`,
		`page: "network"`,
		`page: "simulation"`,
		`visible_presets`,
		`f017FeatureNavigationChanged()`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("Feature workspace integration missing contract marker %q", marker)
		}
	}

	for _, marker := range []string{
		`function f017RegisterFeaturePage`,
		`function f017InsertAfter`,
		`profile.visible_pages = f017InsertAfter`,
		`profile.page_order = f017InsertAfter`,
		`f017ApplyProfile({ navigateIfNeeded: false })`,
	} {
		if !strings.Contains(core, marker) {
			t.Fatalf("Workspace profile core missing feature-registration marker %q", marker)
		}
	}

	for _, source := range []struct {
		name string
		text string
	}{
		{"feature integration", js},
		{"workspace profile core", core},
	} {
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
			if strings.Contains(source.text, forbidden) {
				t.Fatalf("%s must remain presentation-only; found %q", source.name, forbidden)
			}
		}
	}
}
