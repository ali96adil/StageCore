package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedSecurityOperatorClientIsLocalOnly(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootReq.RemoteAddr = "127.0.0.1:17101"
	rootRes := httptest.NewRecorder()
	handler.ServeHTTP(rootRes, rootReq)
	if rootRes.Code != http.StatusOK {
		t.Fatalf("root status=%d", rootRes.Code)
	}
	root := rootRes.Body.String()
	if !strings.Contains(root, `id="securityNav"`) || !strings.Contains(root, `src="/security.js"`) {
		t.Fatal("Operator root is missing Security navigation/client")
	}

	jsReq := httptest.NewRequest(http.MethodGet, "/security.js", nil)
	jsReq.RemoteAddr = "127.0.0.1:17102"
	jsRes := httptest.NewRecorder()
	handler.ServeHTTP(jsRes, jsReq)
	if jsRes.Code != http.StatusOK || !strings.HasPrefix(jsRes.Header().Get("Content-Type"), "application/javascript") {
		t.Fatalf("security.js status=%d content-type=%q", jsRes.Code, jsRes.Header().Get("Content-Type"))
	}
	js := jsRes.Body.String()
	for _, required := range []string{
		"/api/v1/security/secrets",
		"/api/v1/security/users",
		"/api/v1/security/plugins/permissions",
		"/api/v1/security/audit",
		"/api/v1/auth/renew",
		"REVOKE",
	} {
		if !strings.Contains(js, required) {
			t.Fatalf("security.js missing %q", required)
		}
	}
	if strings.Contains(js, "https://") || strings.Contains(js, "http://") {
		t.Fatal("Security operator client must not depend on WAN assets")
	}
}
