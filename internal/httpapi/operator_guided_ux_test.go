package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGuidedOperatorUXAssetsAreEmbeddedAndOffline(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootReq.RemoteAddr = "127.0.0.1:17101"
	rootRes := httptest.NewRecorder()
	handler.ServeHTTP(rootRes, rootReq)
	if rootRes.Code != http.StatusOK {
		t.Fatalf("operator root status=%d body=%s", rootRes.Code, rootRes.Body.String())
	}
	body := rootRes.Body.String()
	if !strings.Contains(body, `href="/guided-ux.css"`) || !strings.Contains(body, `src="/guided-ux.js"`) {
		t.Fatal("operator root does not load F-002 guided UX assets")
	}

	jsReq := httptest.NewRequest(http.MethodGet, "/guided-ux.js", nil)
	jsReq.RemoteAddr = "127.0.0.1:17102"
	jsRes := httptest.NewRecorder()
	handler.ServeHTTP(jsRes, jsReq)
	if jsRes.Code != http.StatusOK || !strings.HasPrefix(jsRes.Header().Get("Content-Type"), "application/javascript") {
		t.Fatalf("guided-ux.js status=%d content-type=%q", jsRes.Code, jsRes.Header().Get("Content-Type"))
	}
	js := jsRes.Body.String()
	for _, required := range []string{
		"RECOMMENDED NEXT STEP",
		"Quick target setup",
		"Send OSC message",
		"Advanced action settings",
		"data.i18n",
	} {
		if !strings.Contains(js, required) {
			t.Fatalf("guided-ux.js missing %q", required)
		}
	}
	if strings.Contains(js, "https://") || strings.Contains(js, "http://") {
		t.Fatal("guided UX must not depend on remote assets or services")
	}

	cssReq := httptest.NewRequest(http.MethodGet, "/guided-ux.css", nil)
	cssReq.RemoteAddr = "127.0.0.1:17103"
	cssRes := httptest.NewRecorder()
	handler.ServeHTTP(cssRes, cssReq)
	if cssRes.Code != http.StatusOK || !strings.HasPrefix(cssRes.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("guided-ux.css status=%d content-type=%q", cssRes.Code, cssRes.Header().Get("Content-Type"))
	}
	css := cssRes.Body.String()
	if !strings.Contains(css, "margin-block") || !strings.Contains(css, "text-align: start") {
		t.Fatal("guided UX styles must use logical/RTL-ready layout primitives")
	}
}
