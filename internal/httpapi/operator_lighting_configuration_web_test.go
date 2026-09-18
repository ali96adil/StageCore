package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperatorWebBundlesLightingSetupWorkspace(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()
	for _, tc := range []struct {
		path     string
		required []string
	}{
		{
			path: "/phase4.js",
			required: []string{
				"renderLightingSetup",
				"lighting-controller/configuration",
				"apply-published-config",
				"/identify",
				"lighting-channel-row",
			},
		},
		{
			path: "/phase4.css",
			required: []string{
				"lighting-config-card",
				"lighting-health-grid",
				"lighting-channel-editor",
				"lighting-health-badge",
			},
		},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.RemoteAddr = "127.0.0.1:19504"
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", tc.path, res.Code, res.Body.String())
		}
		body := res.Body.String()
		for _, required := range tc.required {
			if !strings.Contains(body, required) {
				t.Fatalf("%s missing %q", tc.path, required)
			}
		}
	}
}
