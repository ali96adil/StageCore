package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedOperatorWebIsOfflineAndSecurityBound(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootReq.RemoteAddr = "127.0.0.1:17001"
	rootRes := httptest.NewRecorder()
	handler.ServeHTTP(rootRes, rootReq)
	if rootRes.Code != http.StatusOK {
		t.Fatalf("operator root status=%d body=%s", rootRes.Code, rootRes.Body.String())
	}
	if got := rootRes.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("root content type=%q", got)
	}
	if !strings.Contains(rootRes.Body.String(), "StageCore Operator") ||
		!strings.Contains(rootRes.Body.String(), `href="/app.css"`) ||
		!strings.Contains(rootRes.Body.String(), `src="/app.js"`) ||
		!strings.Contains(rootRes.Body.String(), `src="/preflight.js"`) ||
		!strings.Contains(rootRes.Body.String(), `src="/memory.js"`) ||
		!strings.Contains(rootRes.Body.String(), `data-page="preflight"`) ||
		!strings.Contains(rootRes.Body.String(), `data-page="sessions"`) ||
		!strings.Contains(rootRes.Body.String(), `data-page="notes"`) {
		t.Fatalf("operator root does not reference embedded local Operator navigation")
	}
	if strings.Contains(rootRes.Body.String(), "http://") || strings.Contains(rootRes.Body.String(), "https://") {
		t.Fatal("operator root must not depend on remote web assets")
	}
	if got := rootRes.Header().Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'self'") || !strings.Contains(got, "frame-ancestors 'none'") {
		t.Fatalf("missing restrictive CSP: %q", got)
	}
	if rootRes.Header().Get("X-Content-Type-Options") != "nosniff" || rootRes.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("missing browser security headers: %#v", rootRes.Header())
	}

	jsReq := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	jsReq.RemoteAddr = "127.0.0.1:17002"
	jsRes := httptest.NewRecorder()
	handler.ServeHTTP(jsRes, jsReq)
	if jsRes.Code != http.StatusOK || !strings.HasPrefix(jsRes.Header().Get("Content-Type"), "application/javascript") {
		t.Fatalf("app.js status=%d content-type=%q", jsRes.Code, jsRes.Header().Get("Content-Type"))
	}
	if !strings.Contains(jsRes.Body.String(), "/api/v1/auth/login") || !strings.Contains(jsRes.Body.String(), "/runtime/go") {
		t.Fatal("embedded Operator JS is missing authenticated operator flows")
	}

	preflightReq := httptest.NewRequest(http.MethodGet, "/preflight.js", nil)
	preflightReq.RemoteAddr = "127.0.0.1:17003"
	preflightRes := httptest.NewRecorder()
	handler.ServeHTTP(preflightRes, preflightReq)
	if preflightRes.Code != http.StatusOK || !strings.HasPrefix(preflightRes.Header().Get("Content-Type"), "application/javascript") {
		t.Fatalf("preflight.js status=%d content-type=%q", preflightRes.Code, preflightRes.Header().Get("Content-Type"))
	}
	if !strings.Contains(preflightRes.Body.String(), "/preflight") || !strings.Contains(preflightRes.Body.String(), "PASS / WARN / BLOCK") {
		t.Fatal("embedded Preflight client is missing authoritative readiness flow")
	}

	memoryReq := httptest.NewRequest(http.MethodGet, "/memory.js", nil)
	memoryReq.RemoteAddr = "127.0.0.1:17004"
	memoryRes := httptest.NewRecorder()
	handler.ServeHTTP(memoryRes, memoryReq)
	if memoryRes.Code != http.StatusOK || !strings.HasPrefix(memoryRes.Header().Get("Content-Type"), "application/javascript") {
		t.Fatalf("memory.js status=%d content-type=%q", memoryRes.Code, memoryRes.Header().Get("Content-Type"))
	}
	if !strings.Contains(memoryRes.Body.String(), "/sessions") || !strings.Contains(memoryRes.Body.String(), "/notes") || !strings.Contains(memoryRes.Body.String(), "execution trace") {
		t.Fatal("embedded Session Memory client is missing structured session and note flows")
	}

	cssReq := httptest.NewRequest(http.MethodGet, "/app.css", nil)
	cssReq.RemoteAddr = "127.0.0.1:17005"
	cssRes := httptest.NewRecorder()
	handler.ServeHTTP(cssRes, cssReq)
	if cssRes.Code != http.StatusOK || !strings.HasPrefix(cssRes.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("app.css status=%d content-type=%q", cssRes.Code, cssRes.Header().Get("Content-Type"))
	}

	lanReq := httptest.NewRequest(http.MethodGet, "/", nil)
	lanReq.RemoteAddr = "10.20.30.40:17006"
	lanRes := httptest.NewRecorder()
	handler.ServeHTTP(lanRes, lanReq)
	if lanRes.Code != http.StatusUpgradeRequired || !strings.Contains(lanRes.Body.String(), "SECURE_TRANSPORT_REQUIRED") {
		t.Fatalf("insecure LAN operator UI status=%d body=%s", lanRes.Code, lanRes.Body.String())
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/not-a-stagecore-route", nil)
	missingReq.RemoteAddr = "127.0.0.1:17007"
	missingRes := httptest.NewRecorder()
	handler.ServeHTTP(missingRes, missingReq)
	if missingRes.Code != http.StatusNotFound {
		t.Fatalf("unknown operator path status=%d, want 404", missingRes.Code)
	}
}
