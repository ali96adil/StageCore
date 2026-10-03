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
		!strings.Contains(rootRes.Body.String(), `href="/phase4.css"`) ||
		!strings.Contains(rootRes.Body.String(), `src="/phase4.js"`) ||
		!strings.Contains(rootRes.Body.String(), `src="/phase4-polish.js"`) ||
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
	if !strings.Contains(jsRes.Body.String(), "/api/v1/auth/login") ||
		!strings.Contains(jsRes.Body.String(), "/runtime/go") ||
		!strings.Contains(jsRes.Body.String(), "/runtime/project-blackout") ||
		!strings.Contains(jsRes.Body.String(), "FORCE_EXIT_WITHOUT_BLACKOUT") ||
		!strings.Contains(jsRes.Body.String(), "forceStopSessionButton") {
		t.Fatal("embedded Operator JS is missing authenticated runtime safety/override flows")
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

	phase4Req := httptest.NewRequest(http.MethodGet, "/phase4.js", nil)
	phase4Req.RemoteAddr = "127.0.0.1:17005"
	phase4Res := httptest.NewRecorder()
	handler.ServeHTTP(phase4Res, phase4Req)
	if phase4Res.Code != http.StatusOK || !strings.HasPrefix(phase4Res.Header().Get("Content-Type"), "application/javascript") {
		t.Fatalf("phase4.js status=%d content-type=%q", phase4Res.Code, phase4Res.Header().Get("Content-Type"))
	}
	if !strings.Contains(phase4Res.Body.String(), "/stage-devices") || !strings.Contains(phase4Res.Body.String(), "/live-video-sources") || !strings.Contains(phase4Res.Body.String(), "/network/cockpit") {
		t.Fatal("embedded Phase 4 client is missing Stage Device, live-video or network workflows")
	}

	polishReq := httptest.NewRequest(http.MethodGet, "/phase4-polish.js", nil)
	polishReq.RemoteAddr = "127.0.0.1:17006"
	polishRes := httptest.NewRecorder()
	handler.ServeHTTP(polishRes, polishReq)
	if polishRes.Code != http.StatusOK || !strings.HasPrefix(polishRes.Header().Get("Content-Type"), "application/javascript") {
		t.Fatalf("phase4-polish.js status=%d content-type=%q", polishRes.Code, polishRes.Header().Get("Content-Type"))
	}
	if !strings.Contains(polishRes.Body.String(), "/stage-device-commands") ||
		!strings.Contains(polishRes.Body.String(), "VIDEO_SOURCE_OPEN") ||
		!strings.Contains(polishRes.Body.String(), "VIDEO_SOURCE_INSPECT") {
		t.Fatal("embedded Phase 4 polish client is missing grouped Callboard or live-video controls")
	}

	cssReq := httptest.NewRequest(http.MethodGet, "/phase4.css", nil)
	cssReq.RemoteAddr = "127.0.0.1:17007"
	cssRes := httptest.NewRecorder()
	handler.ServeHTTP(cssRes, cssReq)
	if cssRes.Code != http.StatusOK || !strings.HasPrefix(cssRes.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("phase4.css status=%d content-type=%q", cssRes.Code, cssRes.Header().Get("Content-Type"))
	}

	lanReq := httptest.NewRequest(http.MethodGet, "/", nil)
	lanReq.RemoteAddr = "10.20.30.40:17008"
	lanRes := httptest.NewRecorder()
	handler.ServeHTTP(lanRes, lanReq)
	if lanRes.Code != http.StatusUpgradeRequired || !strings.Contains(lanRes.Body.String(), "SECURE_TRANSPORT_REQUIRED") {
		t.Fatalf("insecure LAN operator UI status=%d body=%s", lanRes.Code, lanRes.Body.String())
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/not-a-stagecore-route", nil)
	missingReq.RemoteAddr = "127.0.0.1:17009"
	missingRes := httptest.NewRecorder()
	handler.ServeHTTP(missingRes, missingReq)
	if missingRes.Code != http.StatusNotFound {
		t.Fatalf("unknown operator path status=%d, want 404", missingRes.Code)
	}
}


func TestOperatorSidebarKeepsInjectedNavigationScrollable(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()

	req := httptest.NewRequest(http.MethodGet, "/app.css", nil)
	req.RemoteAddr = "127.0.0.1:17010"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("app.css status=%d body=%s", res.Code, res.Body.String())
	}
	css := res.Body.String()
	sidebarStart := strings.Index(css, ".sidebar {")
	if sidebarStart < 0 {
		t.Fatal("operator app.css is missing .sidebar")
	}
	sidebarEnd := strings.Index(css[sidebarStart:], "}")
	if sidebarEnd < 0 {
		t.Fatal("operator .sidebar rule is incomplete")
	}
	sidebar := css[sidebarStart : sidebarStart+sidebarEnd]
	if !strings.Contains(sidebar, "overflow-y: auto;") {
		t.Fatal("operator sidebar must scroll independently when injected navigation exceeds the viewport")
	}
	if !strings.Contains(sidebar, "overscroll-behavior: contain;") {
		t.Fatal("operator sidebar must contain wheel/touch overscroll")
	}
}

func TestOperatorCuesDistinguishesNoDraftFromValidationFailure(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()

	req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	req.RemoteAddr = "127.0.0.1:17011"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("app.js status=%d body=%s", res.Code, res.Body.String())
	}
	js := res.Body.String()
	for _, contract := range []string{
		`const hasDraft = payload.revision?.status === "DRAFT";`,
		`No unpublished Draft`,
		`createDraftButton`,
		`/configuration/draft`,
		`Create a Draft to make Cue changes.`,
	} {
		if !strings.Contains(js, contract) {
			t.Errorf("Cue workspace missing no-Draft UX contract %q", contract)
		}
	}
}
