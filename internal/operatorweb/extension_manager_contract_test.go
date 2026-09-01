package operatorweb

import (
	"os"
	"strings"
	"testing"
)

func TestExtensionManagerUsesExistingLifecycleAPIsAndRoleBoundary(t *testing.T) {
	indexBytes, err := os.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	jsBytes, err := os.ReadFile("static/extensions.js")
	if err != nil {
		t.Fatalf("read extensions.js: %v", err)
	}
	cssBytes, err := os.ReadFile("static/extensions.css")
	if err != nil {
		t.Fatalf("read extensions.css: %v", err)
	}
	index := string(indexBytes)
	js := string(jsBytes)
	css := string(cssBytes)

	for _, marker := range []string{
		`href="/extensions.css"`,
		`id="extensionsNav"`,
		`src="/extensions.js"`,
	} {
		if !strings.Contains(index, marker) {
			t.Fatalf("Operator shell missing F-015 marker %q", marker)
		}
	}

	for _, marker := range []string{
		`state.user?.role === "OWNER" || state.user?.role === "TECHNICIAN"`,
		`api("/api/v1/extensions")`,
		`api("/api/v1/extensions/installations")`,
		`/permission-review`,
		`/readiness`,
		`/runtime`,
		`/install-plan`,
		`/install`,
		`/permissions/`,
		`data-transition="${enabled ? "disable" : "enable"}"`,
		`for (const step of steps)`,
		`pkg.production_ready`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("F-015 Manager missing lifecycle/safety marker %q", marker)
		}
	}

	for _, forbidden := range []string{
		`extensions.update`,
		`extensions.repair`,
		`extensions.uninstall`,
		`f015-update`,
		`f015-repair`,
		`f015-uninstall`,
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("F-015 Manager exposes unsupported lifecycle control %q", forbidden)
		}
	}

	for _, marker := range []string{
		`html[dir="rtl"]`,
		`@media (max-width: 900px)`,
		`@media (max-width: 620px)`,
		`grid-template-columns: 1fr`,
	} {
		if !strings.Contains(css, marker) {
			t.Fatalf("F-015 responsive/RTL CSS missing %q", marker)
		}
	}
}

func TestExtensionManagerOwnsArabicAndEnglishKeyedCopy(t *testing.T) {
	jsBytes, err := os.ReadFile("static/extensions.js")
	if err != nil {
		t.Fatalf("read extensions.js: %v", err)
	}
	js := string(jsBytes)

	for _, key := range []string{
		"extensions.nav",
		"extensions.title",
		"extensions.summary",
		"extensions.trust",
		"extensions.production_ready",
		"extensions.install_plan",
		"extensions.permission_review",
		"extensions.readiness",
		"extensions.runtime",
		"extensions.generation",
		"extensions.show_note",
	} {
		if !strings.Contains(js, `"`+key+`"`) {
			t.Fatalf("F-015 keyed copy missing %q", key)
		}
	}
	if !strings.Contains(js, `"ar-IQ":`) || !strings.Contains(js, `en:`) {
		t.Fatal("F-015 Manager must own both Arabic and English copy")
	}
	if strings.Contains(js, `"extensions.blockers": { en: "Blockers", "ar-IQ": "الحواجب" }`) {
		t.Fatal("F-015 Arabic blocker label must use the operational meaning, not the eyebrow homonym")
	}
}
