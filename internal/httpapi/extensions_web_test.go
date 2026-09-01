package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperatorExtensionManagerAssetsAreServedFromEmbeddedHub(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()

	for _, test := range []struct {
		path        string
		contentType string
		marker      string
	}{
		{path: "/extensions.css", contentType: "text/css", marker: ".f015-summary-grid"},
		{path: "/extensions.js", contentType: "application/javascript", marker: "renderExtensions"},
		{path: "/extensions-uninstall.js", contentType: "application/javascript", marker: "f015ExecuteUninstall"},
	} {
		req := httptest.NewRequest(http.MethodGet, test.path, nil)
		req.RemoteAddr = "127.0.0.1:17411"
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d body=%s", test.path, res.Code, res.Body.String())
		}
		if !strings.HasPrefix(res.Header().Get("Content-Type"), test.contentType) {
			t.Fatalf("GET %s content-type=%q, want prefix %q", test.path, res.Header().Get("Content-Type"), test.contentType)
		}
		if !strings.Contains(res.Body.String(), test.marker) {
			t.Fatalf("GET %s missing marker %q", test.path, test.marker)
		}
	}
}

func TestOperatorExtensionUninstallAssetPreservesSafetyGuardrails(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()

	indexReq := httptest.NewRequest(http.MethodGet, "/", nil)
	indexReq.RemoteAddr = "127.0.0.1:17412"
	indexRes := httptest.NewRecorder()
	handler.ServeHTTP(indexRes, indexReq)
	if indexRes.Code != http.StatusOK {
		t.Fatalf("GET / status=%d body=%s", indexRes.Code, indexRes.Body.String())
	}
	indexBody := indexRes.Body.String()
	basePosition := strings.Index(indexBody, `<script src="/extensions.js" defer></script>`)
	uninstallPosition := strings.Index(indexBody, `<script src="/extensions-uninstall.js" defer></script>`)
	if basePosition < 0 || uninstallPosition < 0 || uninstallPosition <= basePosition {
		t.Fatalf("extension uninstall asset must load after base manager: base=%d uninstall=%d", basePosition, uninstallPosition)
	}

	assetReq := httptest.NewRequest(http.MethodGet, "/extensions-uninstall.js", nil)
	assetReq.RemoteAddr = "127.0.0.1:17413"
	assetRes := httptest.NewRecorder()
	handler.ServeHTTP(assetRes, assetReq)
	if assetRes.Code != http.StatusOK {
		t.Fatalf("GET /extensions-uninstall.js status=%d body=%s", assetRes.Code, assetRes.Body.String())
	}
	body := assetRes.Body.String()
	for _, marker := range []string{
		`method: "DELETE"`,
		`/api/v1/extensions/installations/`,
		`globalThis.confirm`,
		`runtime.desired_state === "DISABLED"`,
		`runtime.observed_state === "STOPPED"`,
		`EXTENSION_REQUIRED_BY_INSTALLED`,
		`SHOW_CONFIGURATION_LOCKED`,
		`إلغاء التثبيت`,
	} {
		if !strings.Contains(body, marker) {
			t.Fatalf("extensions-uninstall.js missing safety marker %q", marker)
		}
	}
}
