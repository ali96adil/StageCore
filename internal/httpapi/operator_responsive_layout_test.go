package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperatorResponsiveLayoutRegressionGuardrails(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()

	workspaceReq := httptest.NewRequest(http.MethodGet, "/workspace-profile.css", nil)
	workspaceReq.RemoteAddr = "127.0.0.1:17301"
	workspaceRes := httptest.NewRecorder()
	handler.ServeHTTP(workspaceRes, workspaceReq)
	if workspaceRes.Code != http.StatusOK {
		t.Fatalf("workspace-profile.css status=%d body=%s", workspaceRes.Code, workspaceRes.Body.String())
	}
	workspaceCSS := workspaceRes.Body.String()
	for _, required := range []string{
		"height: auto;",
		"min-height: 64px;",
		"flex-wrap: wrap;",
		"margin-inline-start: auto;",
		"@media (max-width: 1300px)",
		".hub-badge",
		"display: none;",
	} {
		if !strings.Contains(workspaceCSS, required) {
			t.Fatalf("workspace-profile.css missing responsive guardrail %q", required)
		}
	}

	guidedReq := httptest.NewRequest(http.MethodGet, "/guided-ux.css", nil)
	guidedReq.RemoteAddr = "127.0.0.1:17302"
	guidedRes := httptest.NewRecorder()
	handler.ServeHTTP(guidedRes, guidedReq)
	if guidedRes.Code != http.StatusOK {
		t.Fatalf("guided-ux.css status=%d body=%s", guidedRes.Code, guidedRes.Body.String())
	}
	guidedCSS := guidedRes.Body.String()
	for _, required := range []string{
		`.grid.cards:has(#targetForm)`,
		`minmax(min(420px, 100%), 1fr)`,
		`.f002-builder-head > div`,
		`overflow-wrap: anywhere;`,
	} {
		if !strings.Contains(guidedCSS, required) {
			t.Fatalf("guided-ux.css missing Setup guardrail %q", required)
		}
	}
}
