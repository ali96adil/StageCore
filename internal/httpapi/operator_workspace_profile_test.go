package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperatorWorkspaceProfileAssetsAreEmbedded(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootReq.RemoteAddr = "127.0.0.1:17401"
	rootRes := httptest.NewRecorder()
	handler.ServeHTTP(rootRes, rootReq)
	if rootRes.Code != http.StatusOK {
		t.Fatalf("operator root status=%d body=%s", rootRes.Code, rootRes.Body.String())
	}
	body := rootRes.Body.String()
	for _, required := range []string{
		`href="/workspace-profile.css"`,
		`src="/workspace-profile.js"`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("operator root missing %q", required)
		}
	}

	for _, asset := range []struct {
		path        string
		contentType string
		marker      string
	}{
		{path: "/workspace-profile.css", contentType: "text/css", marker: ".f017-profile-control"},
		{path: "/workspace-profile.js", contentType: "application/javascript", marker: "F017_CONTRACT_VERSION"},
	} {
		req := httptest.NewRequest(http.MethodGet, asset.path, nil)
		req.RemoteAddr = "127.0.0.1:17402"
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", asset.path, res.Code, res.Body.String())
		}
		if !strings.HasPrefix(res.Header().Get("Content-Type"), asset.contentType) {
			t.Fatalf("%s content-type=%q", asset.path, res.Header().Get("Content-Type"))
		}
		if !strings.Contains(res.Body.String(), asset.marker) {
			t.Fatalf("%s missing marker %q", asset.path, asset.marker)
		}
	}
}
