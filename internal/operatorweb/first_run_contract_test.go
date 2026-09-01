package operatorweb

import (
	"os"
	"strings"
	"testing"
)

func TestFirstRunWizardUsesAuthoritativeEligibilityAndExistingProjectAPI(t *testing.T) {
	indexBytes, err := os.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	jsBytes, err := os.ReadFile("static/first-run.js")
	if err != nil {
		t.Fatalf("read first-run.js: %v", err)
	}
	cssBytes, err := os.ReadFile("static/first-run.css")
	if err != nil {
		t.Fatalf("read first-run.css: %v", err)
	}
	index := string(indexBytes)
	js := string(jsBytes)
	css := string(cssBytes)

	for _, marker := range []string{
		`href="/first-run.css"`,
		`src="/first-run.js"`,
	} {
		if !strings.Contains(index, marker) {
			t.Fatalf("Operator shell missing F-008 asset %q", marker)
		}
	}

	for _, marker := range []string{
		`state.user?.role === "OWNER"`,
		`state.hub?.bootstrap_state === "CLAIMED"`,
		`state.projects.length === 0`,
		`f008DismissedThisSession`,
		`api("/api/v1/projects"`,
		`method: "POST"`,
		`sessionStorage.setItem(F008_RESUME_PROJECT_KEY, projectID)`,
		`await navigate("configuration")`,
		`localStorage.setItem(F008_LOCALE_KEY`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("F-008 wizard missing safety/workflow marker %q", marker)
		}
	}

	for _, forbidden := range []string{
		`stagecore_f008_complete`,
		`/api/v1/runtime`,
		`/runtime/`,
		`/publish`,
		`setup-code`,
		`setup_code`,
		`/api/v1/security`,
		`/api/v1/secrets`,
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("F-008 wizard unexpectedly crosses deferred/security boundary %q", forbidden)
		}
	}

	for _, marker := range []string{
		`margin-inline-start`,
		`@media (max-width: 700px)`,
		`grid-template-columns: 1fr`,
	} {
		if !strings.Contains(css, marker) {
			t.Fatalf("F-008 responsive/RTL CSS missing %q", marker)
		}
	}
}

func TestFirstRunWizardOwnsArabicAndEnglishKeyedCopy(t *testing.T) {
	jsBytes, err := os.ReadFile("static/first-run.js")
	if err != nil {
		t.Fatalf("read first-run.js: %v", err)
	}
	js := string(jsBytes)

	for _, key := range []string{
		"first_run.title",
		"first_run.summary",
		"first_run.identity_title",
		"first_run.preferences_title",
		"first_run.project_title",
		"first_run.create_continue",
		"first_run.handoff",
	} {
		if !strings.Contains(js, `"`+key+`"`) {
			t.Fatalf("F-008 keyed copy missing %q", key)
		}
	}
	if !strings.Contains(js, `"ar-IQ":`) || !strings.Contains(js, `en:`) {
		t.Fatal("F-008 wizard must own both Arabic and English copy")
	}
}
