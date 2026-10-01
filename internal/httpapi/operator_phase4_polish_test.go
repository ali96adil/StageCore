package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPhase4OperatorPolishExposesGuidedCallboardPresentationControls(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()
	req := httptest.NewRequest(http.MethodGet, "/phase4-polish.js", nil)
	req.RemoteAddr = "127.0.0.1:19106"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("phase4-polish.js status=%d body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	for _, required := range []string{
		"callboardPreset",
		"callboardTargetAt",
		"callboardAlertRole",
		"callboardAlertIntensity",
		"callboardAlertMotion",
		"callboardAlertDuration",
		"callboardChime",
		"phase4BatchResults",
		"intensity_percent",
		"duration_seconds",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("phase4-polish.js missing guided Callboard contract %q", required)
		}
	}
}


func TestPhase4LiveVideoUXSupportsEditingAndMachineRolePlacement(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()
	req := httptest.NewRequest(http.MethodGet, "/phase4.js", nil)
	req.RemoteAddr = "127.0.0.1:19107"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("phase4.js status=%d body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	for _, required := range []string{
		"liveSourcePlacement",
		"execution_machine_role_id",
		"machine-roles",
		"liveSourceExecutionCapabilities",
		"video.source.open",
		"video.source.route",
		"liveSourceRoles",
		"liveSourceExecutionCapabilities.every",
		"required.has(capability)",
		"live-source-edit",
		"live-source-toggle",
		"Update source",
		"Disable",
		"Enable",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("phase4.js missing Live Video UX contract %q", required)
		}
	}
}
