package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The Hub serves a fixed allowlist of embedded Operator assets. Having files
// in go:embed and script tags in index.html is not sufficient: both HTTP
// handlers must exist, with correct MIME and the existing secure-browser gate.
func TestCueStudioAssetsServedOfflineWithSecureBrowserBoundary(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()
	for _, item := range []struct {
	    path, mediaType, marker string
	}{
	    {"/cue-studio.js", "application/javascript", "sc008OpenCueEditor"},
	    {"/cue-studio.css", "text/css", ".sc008-dialog"},
	} {
	    req := httptest.NewRequest(http.MethodGet, item.path, nil)
	    req.RemoteAddr = "127.0.0.1:12345"
	    res := httptest.NewRecorder()
	    handler.ServeHTTP(res, req)
	    if res.Code != http.StatusOK {
	        t.Fatalf("%s status=%d: %s", item.path, res.Code, res.Body.String())
	    }
	    if !strings.HasPrefix(res.Header().Get("Content-Type"), item.mediaType) {
	        t.Fatalf("%s unexpected content type: %s", item.path, res.Header().Get("Content-Type"))
	    }
	    if !strings.Contains(res.Body.String(), item.marker) {
	        t.Fatalf("%s missing expected embedded Cue Studio code", item.path)
	    }
	    if res.Header().Get("Cache-Control") != "no-store" {
	        t.Fatalf("%s must not be cached across show software updates", item.path)
	    }
	    if !strings.Contains(res.Header().Get("Content-Security-Policy"), "script-src 'self'") {
	        t.Fatalf("%s must retain secure browser policy", item.path)
	    }

	    lanReq := httptest.NewRequest(http.MethodGet, item.path, nil)
	    lanReq.RemoteAddr = "10.2.3.4:12345"
	    lanRes := httptest.NewRecorder()
	    handler.ServeHTTP(lanRes, lanReq)
	    if lanRes.Code != http.StatusUpgradeRequired {
	        t.Fatalf("%s insecure LAN status=%d, want secure transport enforcement", item.path, lanRes.Code)
	    }
	}
}
