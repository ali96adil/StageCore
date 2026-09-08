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
	index, err := Read("index.html")
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
	navSource := string(nav)
	for _, required := range []string{
		`dataset.page = "capsules"`,
		`renderGlobalShowCapsuleLibrary`,
		`state.project ? renderShowCapsuleWorkspace : renderGlobalShowCapsuleLibrary`,
		`/api/v1/show-capsules/imports/`,
		`/materialize`,
	} {
		if !strings.Contains(navSource, required) {
			t.Errorf("zero-project Show Capsule navigation missing contract marker %q", required)
		}
	}
	indexSource := string(index)
	if !strings.Contains(indexSource, `<script src="/show-capsules.js" defer></script>`) ||
		!strings.Contains(indexSource, `<script src="/show-capsules-nav.js" defer></script>`) {
		t.Fatal("Operator shell does not load Show Capsule workspace scripts")
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
