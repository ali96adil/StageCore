package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGuidedUXBundleServesDeviceProfileWorkflowAfterF002(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()
	req := httptest.NewRequest(http.MethodGet, "/guided-ux.js", nil)
	req.RemoteAddr = "127.0.0.1:17421"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("GET /guided-ux.js status=%d body=%s", res.Code, res.Body.String())
	}
	if !strings.HasPrefix(res.Header().Get("Content-Type"), "application/javascript") {
		t.Fatalf("GET /guided-ux.js content-type=%q", res.Header().Get("Content-Type"))
	}
	body := res.Body.String()
	f002 := strings.Index(body, "f002EnhanceConfiguration")
	f021 := strings.Index(body, "f021EnhanceConfiguration")
	if f002 < 0 || f021 < 0 {
		t.Fatalf("guided bundle missing F-002/F-021 markers: f002=%d f021=%d", f002, f021)
	}
	if f021 <= f002 {
		t.Fatalf("F-021 must load after F-002 guided shell: f002=%d f021=%d", f002, f021)
	}
}
