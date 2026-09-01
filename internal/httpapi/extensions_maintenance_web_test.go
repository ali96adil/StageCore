package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperatorExtensionMaintenanceIsEmbeddedAndComposedAfterUninstall(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()

	bundleReq := httptest.NewRequest(http.MethodGet, "/extensions-uninstall.js", nil)
	bundleReq.RemoteAddr = "127.0.0.1:17414"
	bundleRes := httptest.NewRecorder()
	handler.ServeHTTP(bundleRes, bundleReq)
	if bundleRes.Code != http.StatusOK {
		t.Fatalf("GET /extensions-uninstall.js status=%d body=%s", bundleRes.Code, bundleRes.Body.String())
	}
	body := bundleRes.Body.String()
	uninstallPosition := strings.Index(body, "f015ExecuteUninstall")
	maintenancePosition := strings.Index(body, "f015ShowMaintenancePlan")
	if uninstallPosition < 0 || maintenancePosition < 0 || maintenancePosition <= uninstallPosition {
		t.Fatalf("maintenance must be composed after uninstall definitions: uninstall=%d maintenance=%d", uninstallPosition, maintenancePosition)
	}
	for _, marker := range []string{
		`/update-plan?target_package_id=`,
		`/repair`,
		`extensions.version_reset_note`,
		`runtime.desired_state === "DISABLED" && runtime.observed_state === "STOPPED"`,
	} {
		if !strings.Contains(body, marker) {
			t.Fatalf("extension maintenance bundle missing marker %q", marker)
		}
	}

	directReq := httptest.NewRequest(http.MethodGet, "/extensions-maintenance.js", nil)
	directReq.RemoteAddr = "127.0.0.1:17415"
	directRes := httptest.NewRecorder()
	handler.ServeHTTP(directRes, directReq)
	if directRes.Code != http.StatusOK || !strings.Contains(directRes.Body.String(), "f015ShowMaintenancePlan") {
		t.Fatalf("GET /extensions-maintenance.js status=%d body=%s", directRes.Code, directRes.Body.String())
	}
}
