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

	// The feature-registration shim itself must remain presentation-only.
	// workspace-profile.js already contains its pre-existing GET-only SHOW-lock
	// policy check, so scanning the whole core file for "api(" is a false positive.
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
			t.Fatalf("feature integration must remain presentation-only; found %q", forbidden)
		}
	}

	for _, forbidden := range []string{
		`method: "POST"`,
		`method: "PUT"`,
		`method: "PATCH"`,
		`method: "DELETE"`,
		`/publish`,
	} {
		if strings.Contains(core, forbidden) {
			t.Fatalf("workspace profile registration must not add mutation authority; found %q", forbidden)
		}
	}
}
