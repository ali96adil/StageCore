package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperatorWebBundlesAssistantWorkspace(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()
	for _, tc := range []struct {
		path     string
		required []string
	}{
		{path: "/phase4.js", required: []string{
			"renderAssistantWorkspace",
			"/assistant/status",
			"/assistant/tasks",
			"/assistant/proposals/preview",
			"apply-batch",
			"ASSISTANT_PROVIDER_UNAVAILABLE",
			"data-assistant-nav",
			"assistant.no_auto_apply",
			"DRAFT_PROPOSAL",
			"READ_ONLY",
		}},
		{path: "/phase4.css", required: []string{
			"assistant-task-card",
			"assistant-context-grid",
			"assistant-operation-card",
			"html[dir=\"rtl\"] .assistant-operation-card",
		}},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.RemoteAddr = "127.0.0.1:19223"
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
