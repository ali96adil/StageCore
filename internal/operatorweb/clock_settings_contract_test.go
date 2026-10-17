package operatorweb

import (
	"strings"
	"testing"
)

func TestOfflineHubClockSettingsIsReadOnlyAndAvailableOutsideProject(t *testing.T) {
	html := string(mustReadOperatorContractFile(t, "static/index.html"))
	js := string(mustReadOperatorContractFile(t, "static/clock-settings.js"))
	css := string(mustReadOperatorContractFile(t, "static/app.css"))

	for _, marker := range []string{`id="clockNav"`, `src="/clock-settings.js"`} {
		if !strings.Contains(html, marker) {
			t.Fatalf("missing global Hub clock settings navigation %q", marker)
		}
	}
	for _, marker := range []string{
		`/api/v1/system/clock/status`,
		`state.page === "clock"`,
		`clockMeasure()`,
		`clockOffset`,
		`clockRTC`,
		`clockBattery`,
		`setPage("clock")`,
		`Date.now()`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("missing clock status/phone comparison marker %q", marker)
		}
	}
	for _, disallowed := range []string{
		`/api/v1/system/clock/adjust`,
		`/api/v1/system/time/set`,
		`https://`, `http://`,
	} {
		if strings.Contains(js, disallowed) {
			t.Fatalf("clock monitoring UI must stay read-only and offline: %q", disallowed)
		}
	}
	if !strings.Contains(css, ".clock-status-grid") {
		t.Fatal("clock page must have a responsive layout")
	}
}


func TestClockCorrectionPreviewHasNoApplyCapability(t *testing.T) {
	js := string(mustReadOperatorContractFile(t, "static/clock-settings.js"))
	for _, marker := range []string{
		`clockPreviewProposal("PHONE", Date.now())`,
		`clockPreviewProposal("MANUAL", manual)`,
		`clockBaghdadManualMillis`,
		`/api/v1/system/clock/preview`,
		`clockPreviewResult`,
	} {
		if !strings.Contains(js, marker) {
			t.Fatalf("phone/manual clock preview contract missing %q", marker)
		}
	}
	for _, forbidden := range []string{
		`/api/v1/system/clock/adjust`,
		`/api/v1/system/clock/apply`,
		`/api/v1/system/time/set`,
		`timedatectl set-time`,
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("clock preview must not expose privileged adjustment: %q", forbidden)
		}
	}
}
