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
