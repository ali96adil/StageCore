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
	if !strings.Contains(body, `href="/guided-ux.css"`) ||
		!strings.Contains(body, `src="/show-lock.js"`) ||
		!strings.Contains(body, `src="/guided-ux.js"`) {
		t.Fatal("operator root does not load SHOW lock and F-002 guided UX assets")
	}

	showLockReq := httptest.NewRequest(http.MethodGet, "/show-lock.js", nil)
	showLockReq.RemoteAddr = "127.0.0.1:17102"
	showLockRes := httptest.NewRecorder()
	handler.ServeHTTP(showLockRes, showLockReq)
	if showLockRes.Code != http.StatusOK || !strings.HasPrefix(showLockRes.Header().Get("Content-Type"), "application/javascript") {
		t.Fatalf("show-lock.js status=%d content-type=%q", showLockRes.Code, showLockRes.Header().Get("Content-Type"))
	}
	if !strings.Contains(showLockRes.Body.String(), "SHOW MODE — CONFIGURATION LOCKED") {
		t.Fatal("embedded SHOW lock client is missing the lock banner behavior")
	}

	jsReq := httptest.NewRequest(http.MethodGet, "/guided-ux.js", nil)
	jsReq.RemoteAddr = "127.0.0.1:17103"
	jsRes := httptest.NewRecorder()
	handler.ServeHTTP(jsRes, jsReq)
	if jsRes.Code != http.StatusOK || !strings.HasPrefix(jsRes.Header().Get("Content-Type"), "application/javascript") {
		t.Fatalf("guided-ux.js status=%d content-type=%q", jsRes.Code, jsRes.Header().Get("Content-Type"))
	}
	js := jsRes.Body.String()
	for _, required := range []string{
		"RECOMMENDED NEXT STEP",
		"Quick target setup",
		"Route behavior",
		"f002RouteConditionKind",
		"f002RouteTransformKind",
		"Use transformed input",
		"Inside numeric range",
		"Scale / offset number",
		"Advanced routing conditions and parameters",
		"Send OSC message",
		"Send MIDI message",
		"midi.send",
		"MIDI destination name",
		"Stable destination name",
		"Legacy numeric index",
		"destination_name",
		"MIDI destination index",
		"Note On",
		"Control Change",
		"Program Change",
		"destination_index",
		"f002MIDIStatus",
		"Ableton Live",
		"MIDI Map mode",
		"Reliability",
		"f002ParseTimeoutPolicy",
		"f002ParseErrorPolicy",
		"FAIL_CUE",
		"CONTINUE",
		"Custom timeout (ms)",
		"Advanced Cue policy",
		"SEQUENTIAL, PARALLEL, or PARALLEL_BARRIER",
		"Advanced action settings",
		"dataset.i18n",
	} {
		if !strings.Contains(js, required) {
			t.Fatalf("guided-ux.js missing %q", required)
		}
	}
	if strings.Contains(js, "https://") || strings.Contains(js, "http://") {
		t.Fatal("guided UX must not depend on remote assets or services")
	}

	appReq := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	appReq.RemoteAddr = "127.0.0.1:17105"
	appRes := httptest.NewRecorder()
	handler.ServeHTTP(appRes, appReq)
	if appRes.Code != http.StatusOK {
		t.Fatalf("app.js status=%d", appRes.Code)
	}
	app := appRes.Body.String()
	for _, required := range []string{
		"/preflight",
		"Preflight:",
		"showBlocked",
		"runtimeOpenPreflight",
		"Client readiness display cannot bypass Preflight",
	} {
		if !strings.Contains(app, required) {
			t.Fatalf("app.js missing Runtime Preflight UX contract %q", required)
		}
	}

	cssReq := httptest.NewRequest(http.MethodGet, "/guided-ux.css", nil)
	cssReq.RemoteAddr = "127.0.0.1:17104"
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
