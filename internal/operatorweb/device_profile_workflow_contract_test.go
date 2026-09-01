package operatorweb

import (
	"os"
	"strings"
	"testing"
)

func TestDeviceProfileWorkflowUsesCatalogMaterializationAndNormalProjectTargetWrite(t *testing.T) {
	content, err := os.ReadFile("static/device-profiles.js")
	if err != nil {
		t.Fatalf("read device-profiles.js: %v", err)
	}
	source := string(content)

	for _, marker := range []string{
		`api("/api/v1/device-profiles")`,
		`/materialize`,
		`/api/v1/projects/${encodeURIComponent(state.project.project_id)}/targets`,
		`method: "POST"`,
		`reviewed.target.logical_type`,
		`reviewed.target.configuration`,
		`save.disabled = true`,
		`f021ReadValues(profile, form)`,
		`profile.connection_fields`,
		`profile.capabilities`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("F-021 guided workflow missing contract marker %q", marker)
		}
	}

	for _, forbidden := range []string{
		`JSON.stringify({ osc:`,
		`/api/v1/runtime`,
		`/publish`,
		`setup-code`,
		`setup_code`,
		`/api/v1/security`,
		`/api/v1/secrets`,
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("F-021 guided workflow unexpectedly bypasses profile/project boundary with %q", forbidden)
		}
	}
}

func TestDeviceProfileWorkflowOwnsBilingualKeyedCopy(t *testing.T) {
	content, err := os.ReadFile("static/device-profiles.js")
	if err != nil {
		t.Fatalf("read device-profiles.js: %v", err)
	}
	source := string(content)

	for _, key := range []string{
		"device_profiles.title",
		"device_profiles.summary",
		"device_profiles.profile",
		"device_profiles.logical_name",
		"device_profiles.review",
		"device_profiles.save",
		"device_profiles.review_note",
		"device_profiles.preview_ready",
		"device_profiles.advanced",
		"device_profiles.unavailable",
	} {
		if !strings.Contains(source, `"`+key+`"`) {
			t.Fatalf("F-021 keyed copy missing %q", key)
		}
	}
	if !strings.Contains(source, `"ar-IQ":`) || !strings.Contains(source, `en:`) {
		t.Fatal("F-021 guided workflow must own both Arabic and English copy")
	}
}
