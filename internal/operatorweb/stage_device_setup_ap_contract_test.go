package operatorweb_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/httpapi"
)

func TestStageDeviceSetupAPMaintenanceSurface(t *testing.T) {
	handler := httpapi.New(httpapi.WithOperatorWeb()).Handler()
	req := httptest.NewRequest(http.MethodGet, "/phase4.js", nil)
	req.RemoteAddr = "127.0.0.1:19922"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("phase4.js status=%d body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	for _, required := range []string{
		"device.maintenance.setup-ap-password",
		"data-setup-ap-save",
		"data-setup-ap-reset",
		"stage-setup-ap-password",
		"type=\"password\"",
		"autocomplete=\"new-password\"",
		"/setup-ap-password",
		"reset_to_default",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("phase4.js missing %q", required)
		}
	}
}
