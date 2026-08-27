package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/secretstore"
	"github.com/ali96adil/StageCore/internal/securityaudit"
)

func TestBrowserAuthAuditAndRenewal(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	secrets, err := secretstore.Open(ctx, h.db.DB, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	audit, err := securityaudit.New(h.db.DB, secrets)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithUserAuth(h.auth, h.hub, audit)).Handler()

	wrong := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"owner","password":"wrong"}`))
	wrong.RemoteAddr = "127.0.0.1:4001"
	wrongRes := httptest.NewRecorder()
	handler.ServeHTTP(wrongRes, wrong)
	if wrongRes.Code != http.StatusUnauthorized {
		t.Fatalf("wrong login=%d", wrongRes.Code)
	}

	loginBody, _ := json.Marshal(map[string]string{"username": "owner", "password": h.password})
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	login.RemoteAddr = "127.0.0.1:4001"
	loginRes := httptest.NewRecorder()
	handler.ServeHTTP(loginRes, login)
	if loginRes.Code != http.StatusOK {
		t.Fatalf("login=%d %s", loginRes.Code, loginRes.Body.String())
	}
	var loginPayload struct { CSRF string `json:"csrf_token"` }
	if err := json.Unmarshal(loginRes.Body.Bytes(), &loginPayload); err != nil {
		t.Fatal(err)
	}
	cookies := loginRes.Result().Cookies()
	if len(cookies) != 1 || loginPayload.CSRF == "" {
		t.Fatalf("login cookie/csrf missing")
	}
	oldCookie := cookies[0]

	renew := httptest.NewRequest(http.MethodPost, "/api/v1/auth/renew", nil)
	renew.RemoteAddr = "127.0.0.1:4001"
	renew.AddCookie(oldCookie)
	renew.Header.Set(csrfHeader, loginPayload.CSRF)
	renewRes := httptest.NewRecorder()
	handler.ServeHTTP(renewRes, renew)
	if renewRes.Code != http.StatusOK {
		t.Fatalf("renew=%d %s", renewRes.Code, renewRes.Body.String())
	}
	var renewPayload struct { CSRF string `json:"csrf_token"` }
	if err := json.Unmarshal(renewRes.Body.Bytes(), &renewPayload); err != nil {
		t.Fatal(err)
	}
	newCookies := renewRes.Result().Cookies()
	if len(newCookies) != 1 || newCookies[0].Value == oldCookie.Value || renewPayload.CSRF == "" || renewPayload.CSRF == loginPayload.CSRF {
		t.Fatal("renewal did not rotate cookie and CSRF")
	}

	logout := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logout.RemoteAddr = "127.0.0.1:4001"
	logout.AddCookie(newCookies[0])
	logout.Header.Set(csrfHeader, renewPayload.CSRF)
	logoutRes := httptest.NewRecorder()
	handler.ServeHTTP(logoutRes, logout)
	if logoutRes.Code != http.StatusNoContent {
		t.Fatalf("logout=%d %s", logoutRes.Code, logoutRes.Body.String())
	}

	records, err := audit.List(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	serialized, _ := json.Marshal(records)
	text := string(serialized)
	for _, event := range []string{"auth.login", "auth.session.renew", "auth.logout"} {
		if !strings.Contains(text, event) {
			t.Fatalf("audit missing %s: %s", event, text)
		}
	}
	if !strings.Contains(text, securityaudit.ResultRejected) || !strings.Contains(text, securityaudit.ResultSuccess) {
		t.Fatalf("audit missing login denial/success: %s", text)
	}
}
