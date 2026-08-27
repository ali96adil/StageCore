package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedConfigurationClientUsesOperatorAPIOnly(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()
	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootReq.RemoteAddr = "127.0.0.1:18101"
	rootRes := httptest.NewRecorder()
	handler.ServeHTTP(rootRes, rootReq)
	if rootRes.Code != http.StatusOK || !strings.Contains(rootRes.Body.String(), `data-page="configuration"`) || !strings.Contains(rootRes.Body.String(), `src="/configuration.js"`) {
		t.Fatalf("Operator Configuration navigation missing: %d", rootRes.Code)
	}

	jsReq := httptest.NewRequest(http.MethodGet, "/configuration.js", nil)
	jsReq.RemoteAddr = "127.0.0.1:18102"
	jsRes := httptest.NewRecorder()
	handler.ServeHTTP(jsRes, jsReq)
	if jsRes.Code != http.StatusOK {
		t.Fatalf("configuration.js status=%d", jsRes.Code)
	}
	client := jsRes.Body.String()
	for _, required := range []string{"/configuration", "/targets", "/inputs", "/outputs", "/routes"} {
		if !strings.Contains(client, required) {
			t.Fatalf("configuration client missing %q", required)
		}
	}
	if strings.Contains(client, "http://") || strings.Contains(client, "https://") {
		t.Fatal("Configuration client must remain WAN-independent")
	}
}
