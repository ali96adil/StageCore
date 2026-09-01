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
