package operatorweb_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/httpapi"
)

func TestStageDeviceFirmwareQualifiedUploadSurface(t *testing.T) {
	handler := httpapi.New(httpapi.WithOperatorWeb()).Handler()
	req := httptest.NewRequest(http.MethodGet, "/phase4.js", nil)
	req.RemoteAddr = "127.0.0.1:19921"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("phase4.js status=%d body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	for _, required := range []string{
		"data-firmware-register",
		"stage-firmware-file",
		"stage-firmware-version",
		"stage-firmware-revision",
		"stage-firmware-sha",
		"stage-firmware-qualified",
		`form.append("qualification", "QUALIFIED")`,
		"/firmware-artifacts",
		"40-character lowercase Git revision",
		"64-character lowercase SHA-256",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("phase4.js missing %q", required)
		}
	}
}
