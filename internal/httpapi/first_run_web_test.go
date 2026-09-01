package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperatorFirstRunAssetsAreServedFromEmbeddedHub(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()

	for _, test := range []struct {
		path        string
		contentType string
		marker      string
	}{
		{path: "/first-run.css", contentType: "text/css", marker: ".f008-dialog"},
		{path: "/first-run.js", contentType: "application/javascript", marker: "f008MaybeStart"},
	} {
		req := httptest.NewRequest(http.MethodGet, test.path, nil)
		req.RemoteAddr = "127.0.0.1:17401"
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
