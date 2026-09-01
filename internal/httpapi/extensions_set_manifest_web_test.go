package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperatorExtensionSetManifestAssetIsEmbeddedAndOrdered(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()

	indexReq := httptest.NewRequest(http.MethodGet, "/", nil)
	indexReq.RemoteAddr = "127.0.0.1:19401"
	indexRes := httptest.NewRecorder()
	handler.ServeHTTP(indexRes, indexReq)
	if indexRes.Code != http.StatusOK {
		t.Fatalf("GET / status=%d body=%s", indexRes.Code, indexRes.Body.String())
	}
	body := indexRes.Body.String()
	basePosition := strings.Index(body, `<script src="/extensions.js" defer></script>`)
	setPosition := strings.Index(body, `<script src="/extensions-set-manifest.js" defer></script>`)
	if basePosition < 0 || setPosition < 0 || setPosition <= basePosition {
		t.Fatalf("extension set asset must load after base manager: base=%d set=%d", basePosition, setPosition)
	}

	assetReq := httptest.NewRequest(http.MethodGet, "/extensions-set-manifest.js", nil)
	assetReq.RemoteAddr = "127.0.0.1:19402"
	assetRes := httptest.NewRecorder()
	handler.ServeHTTP(assetRes, assetReq)
	if assetRes.Code != http.StatusOK {
		t.Fatalf("GET /extensions-set-manifest.js status=%d body=%s", assetRes.Code, assetRes.Body.String())
	}
	if !strings.HasPrefix(assetRes.Header().Get("Content-Type"), "application/javascript") {
		t.Fatalf("content-type=%q", assetRes.Header().Get("Content-Type"))
	}
}

func TestOperatorExtensionSetManifestUIPreservesRestoreSafetyContract(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()
	req := httptest.NewRequest(http.MethodGet, "/extensions-set-manifest.js", nil)
	req.RemoteAddr = "127.0.0.1:19403"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("GET asset status=%d body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	for _, marker := range []string{
		`/api/v1/extensions/set-manifest`,
		`/api/v1/extensions/set-manifest/restore-plan`,
		`/api/v1/extensions/set-manifest/restore`,
		`globalThis.confirm`,
		`f015ExtensionSetPlan?.status !== "READY"`,
		`New Plugins remain disabled`,
		`permission approvals will not be restored`,
		`تبقى الإضافات الجديدة معطلة`,
		`لن تتم استعادة موافقات الصلاحيات`,
	} {
		if !strings.Contains(body, marker) {
			t.Fatalf("extensions-set-manifest.js missing safety marker %q", marker)
		}
	}
}
