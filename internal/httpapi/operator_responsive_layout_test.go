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
		`grid-template-columns: 1fr;`,
		`minmax(min(220px, 100%), 1fr)`,
		`white-space: pre-wrap;`,
		`overflow-wrap: anywhere;`,
	} {
		if !strings.Contains(guidedCSS, required) {
			t.Fatalf("guided-ux.css missing Setup guardrail %q", required)
		}
	}

	localizationReq := httptest.NewRequest(http.MethodGet, "/localization.css", nil)
	localizationReq.RemoteAddr = "127.0.0.1:17303"
	localizationRes := httptest.NewRecorder()
	handler.ServeHTTP(localizationRes, localizationReq)
	if localizationRes.Code != http.StatusOK {
		t.Fatalf("localization.css status=%d body=%s", localizationRes.Code, localizationRes.Body.String())
	}
	localizationCSS := localizationRes.Body.String()
	for _, required := range []string{
		`#content:has(#createSecretForm) > .grid.cards`,
		`margin-block-end: 22px;`,
		`#pairingApproveForm`,
		`#companionRevokeForm`,
		`#createUserForm.form-grid.three`,
		`line-height: 1.65;`,
	} {
		if !strings.Contains(localizationCSS, required) {
			t.Fatalf("localization.css missing Security spacing guardrail %q", required)
		}
	}
}
