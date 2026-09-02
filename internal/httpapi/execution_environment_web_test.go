package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedExecutionEnvironmentManagerBundle(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootReq.RemoteAddr = "127.0.0.1:17100"
	rootRes := httptest.NewRecorder()
	handler.ServeHTTP(rootRes, rootReq)
	if rootRes.Code != http.StatusOK {
		t.Fatalf("operator root status=%d body=%s", rootRes.Code, rootRes.Body.String())
	}
	if !strings.Contains(rootRes.Body.String(), `data-page="environments"`) || !strings.Contains(rootRes.Body.String(), `src="/execution-environments.js"`) {
		t.Fatal("operator root is missing the F-025 Environments workspace")
	}

	assetReq := httptest.NewRequest(http.MethodGet, "/execution-environments.js", nil)
	assetReq.RemoteAddr = "127.0.0.1:17101"
	assetRes := httptest.NewRecorder()
	handler.ServeHTTP(assetRes, assetReq)
	if assetRes.Code != http.StatusOK || !strings.HasPrefix(assetRes.Header().Get("Content-Type"), "application/javascript") {
		t.Fatalf("F-025 bundle status=%d content-type=%q", assetRes.Code, assetRes.Header().Get("Content-Type"))
	}
	body := assetRes.Body.String()
	for _, token := range []string{
		"stagecore.adapter.vdmx",
		"workspace.page.environments",
		"f025.capture_file",
		"/vault-status",
		"body: file",
	} {
		if !strings.Contains(body, token) {
			t.Fatalf("F-025 bundle is missing token %q", token)
		}
	}
}
