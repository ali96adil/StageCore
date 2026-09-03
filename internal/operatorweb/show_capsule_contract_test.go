package operatorweb

import (
	"strings"
	"testing"
	"unicode"
)

func TestShowCapsuleWorkspaceBilingualSafetyContract(t *testing.T) {
	workspace, err := Read("show-capsules.js")
	if err != nil {
		t.Fatal(err)
	}
	nav, err := Read("show-capsules-nav.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(workspace)
	for _, required := range []string{
		`"capsule.title"`,
		`"capsule.self_contained"`,
		`"capsule.manifest_only"`,
		`"capsule.materialization_ready"`,
		`"capsule.host_not_ready"`,
		`"capsule.no_overwrite"`,
		`"capsule.show_lock"`,
		`"capsule.extensions_review"`,
		`"capsule.presentation_local"`,
		`/show-capsules/imports/`,
		`/materialize`,
	} {
		if !strings.Contains(source, required) {
			t.Errorf("Show Capsule workspace missing contract marker %q", required)
		}
	}
	if !containsArabicText(source) {
		t.Fatal("Show Capsule workspace has no Arabic operator copy")
	}
	if !strings.Contains(source, `en:`) || !strings.Contains(source, `"ar-IQ":`) {
		t.Fatal("Show Capsule workspace does not expose both en and ar-IQ keyed copy")
	}
	if !strings.Contains(string(nav), `data-page`) && !strings.Contains(string(nav), `dataset.page = "capsules"`) {
		t.Fatal("Show Capsule workspace navigation is not registered")
	}
}

func containsArabicText(value string) bool {
	for _, r := range value {
		if unicode.In(r, unicode.Arabic) {
			return true
		}
	}
	return false
}
