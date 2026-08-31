package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperatorArabicLocalizationIsEmbeddedAndSwitchable(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootReq.RemoteAddr = "127.0.0.1:17201"
	rootRes := httptest.NewRecorder()
	handler.ServeHTTP(rootRes, rootReq)
	if rootRes.Code != http.StatusOK {
		t.Fatalf("operator root status=%d body=%s", rootRes.Code, rootRes.Body.String())
	}
	body := rootRes.Body.String()
	for _, required := range []string{
		`<html lang="ar-IQ" dir="rtl">`,
		`href="/localization.css"`,
		`src="/localization.js"`,
		`id="languageSelect"`,
		`value="ar">العربية`,
		`value="en">English`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("operator root missing %q", required)
		}
	}

	jsReq := httptest.NewRequest(http.MethodGet, "/localization.js", nil)
	jsReq.RemoteAddr = "127.0.0.1:17202"
	jsRes := httptest.NewRecorder()
	handler.ServeHTTP(jsRes, jsReq)
	if jsRes.Code != http.StatusOK || !strings.HasPrefix(jsRes.Header().Get("Content-Type"), "application/javascript") {
		t.Fatalf("localization.js status=%d content-type=%q", jsRes.Code, jsRes.Header().Get("Content-Type"))
	}
	js := jsRes.Body.String()
	for _, required := range []string{
		`F001_DEFAULT_LOCALE = "ar"`,
		`"StageCore Projects": "مشاريع StageCore"`,
		`"PREFLIGHT": "الفحص المسبق"`,
		`"SESSION MEMORY": "سجل الجلسات"`,
		`"HUB SECURITY": "أمان الـ Hub"`,
		`"SHOW MODE — CONFIGURATION LOCKED": "وضع العرض — الإعدادات مقفلة"`,
		`SHOW_CONFIGURATION_LOCKED`,
		`GO — تنفيذ`,
		`MutationObserver`,
		`window.confirm`,
		`window.prompt`,
		`localStorage.setItem(F001_LOCALE_KEY`,
	} {
		if !strings.Contains(js, required) {
			t.Fatalf("localization.js missing %q", required)
		}
	}
	if strings.Contains(js, "https://") || strings.Contains(js, "http://") {
		t.Fatal("localization runtime must not depend on remote services")
	}

	cssReq := httptest.NewRequest(http.MethodGet, "/localization.css", nil)
	cssReq.RemoteAddr = "127.0.0.1:17203"
	cssRes := httptest.NewRecorder()
	handler.ServeHTTP(cssRes, cssReq)
	if cssRes.Code != http.StatusOK || !strings.HasPrefix(cssRes.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("localization.css status=%d content-type=%q", cssRes.Code, cssRes.Header().Get("Content-Type"))
	}
	css := cssRes.Body.String()
	for _, required := range []string{
		`html[dir="rtl"]`,
		`direction: rtl`,
		`direction: ltr`,
		`text-align: start`,
		`margin-inline-start`,
	} {
		if !strings.Contains(css, required) {
			t.Fatalf("localization.css missing %q", required)
		}
	}
}
