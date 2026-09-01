package operatorweb

import (
	"os"
	"strings"
	"testing"
)

func TestExtensionMaintenanceUsesServerPlanAndSafeRuntimeBoundary(t *testing.T) {
	jsBytes, err := os.ReadFile("static/extensions-maintenance.js")
	if err != nil {
		t.Fatalf("read extensions-maintenance.js: %v", err)
	}
	js := string(jsBytes)

	for _, marker := range []string{
		`/update-plan?target_package_id=`,
		`/update`,
		`/repair`,
		`plan.direction === "ROLLBACK"`,
		`result?.direction === "ROLLBACK"`,
		`plan.status === "REQUIRES_DEPENDENCIES"`,
		`for (const step of (plan.steps || []))`,
		`runtime.desired_state === "DISABLED" && runtime.observed_state === "STOPPED"`,
		`extensions.version_reset_note`,
		`target_package_id: targetPackageID`,
		`globalThis.confirm`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("F-015 maintenance UI missing safety marker %q", marker)
		}
	}

	for _, forbidden := range []string{
		`compareSemanticVersions`,
		`localeCompare(installation.version`,
		`parseInt(installation.version`,
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("F-015 maintenance UI must not infer update/rollback direction client-side: %q", forbidden)
		}
	}
}

func TestExtensionMaintenanceOwnsArabicAndEnglishOperatorCopy(t *testing.T) {
	jsBytes, err := os.ReadFile("static/extensions-maintenance.js")
	if err != nil {
		t.Fatalf("read extensions-maintenance.js: %v", err)
	}
	js := string(jsBytes)

	for _, key := range []string{
		"extensions.maintenance",
		"extensions.target_version",
		"extensions.version_reset_note",
		"extensions.update_plan",
		"extensions.update",
		"extensions.rollback",
		"extensions.repair",
		"extensions.update_runtime_required",
		"extensions.update_show_locked",
	} {
		if !strings.Contains(js, `"`+key+`"`) {
			t.Fatalf("F-015 maintenance keyed copy missing %q", key)
		}
	}
	if !strings.Contains(js, `"ar-IQ":`) || !strings.Contains(js, `en:`) {
		t.Fatal("F-015 maintenance UI must own both Arabic and English copy")
	}
	if !strings.Contains(js, "موافقات الصلاحيات") || !strings.Contains(js, "الرجوع") || !strings.Contains(js, "إصلاح") {
		t.Fatal("F-015 maintenance Arabic copy must explain permission reset, rollback and repair")
	}
}
