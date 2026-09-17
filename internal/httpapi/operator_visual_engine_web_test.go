package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperatorWebBundlesVisualEngineWorkspace(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()
	for _, tc := range []struct {
		path     string
		required []string
	}{
		{path: "/phase4.js", required: []string{"renderVisualEngine", "/visual-engine/mode", "SHOW_CONFIGURATION_LOCKED", "data-visual-engine-nav"}},
		{path: "/phase4.css", required: []string{"visual-mode-grid", "visual-inventory-grid", "visual-authority-handoff"}},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.RemoteAddr = "127.0.0.1:19206"
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
