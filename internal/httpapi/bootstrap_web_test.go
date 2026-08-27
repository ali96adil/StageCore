package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedOperatorWebIncludesOfflineFirstOwnerBootstrap(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootReq.RemoteAddr = "127.0.0.1:18101"
	rootRes := httptest.NewRecorder()
	handler.ServeHTTP(rootRes, rootReq)
	if rootRes.Code != http.StatusOK {
		t.Fatalf("root status=%d body=%s", rootRes.Code, rootRes.Body.String())
	}
	root := rootRes.Body.String()
	for _, required := range []string{`id="bootstrapPanel"`, `id="bootstrapForm"`, `src="/bootstrap.js"`, "stagecore-setup setup-code"} {
		if !strings.Contains(root, required) {
			t.Fatalf("Operator root missing bootstrap marker %q", required)
		}
	}

	jsReq := httptest.NewRequest(http.MethodGet, "/bootstrap.js", nil)
	jsReq.RemoteAddr = "127.0.0.1:18102"
	jsRes := httptest.NewRecorder()
	handler.ServeHTTP(jsRes, jsReq)
	if jsRes.Code != http.StatusOK || !strings.HasPrefix(jsRes.Header().Get("Content-Type"), "application/javascript") {
		t.Fatalf("bootstrap.js status=%d content-type=%q", jsRes.Code, jsRes.Header().Get("Content-Type"))
	}
	js := jsRes.Body.String()
	if !strings.Contains(js, "/api/v1/auth/bootstrap") || !strings.Contains(js, "UNCLAIMED") || !strings.Contains(js, "apiLogin") {
		t.Fatal("bootstrap client is missing claim/login flow")
	}
	if strings.Contains(js, "https://") || strings.Contains(js, "http://") {
		t.Fatal("bootstrap client must not depend on WAN assets")
	}
}
