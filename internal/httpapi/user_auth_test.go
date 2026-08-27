package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/hubsecurity"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type authHarness struct {
	db       *db.Handle
	hub      *hubsecurity.Service
	auth     *userauth.Service
	handler  http.Handler
	password string
}

func newAuthHarness(t *testing.T) *authHarness {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	security, err := hubsecurity.Open(ctx, h.DB, root)
	if err != nil {
		t.Fatal(err)
	}
	setup, err := security.GenerateSetupCode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	password := "correct horse battery staple"
	if _, err := security.ClaimFirstOwner(ctx, setup.Code, "owner", password); err != nil {
		t.Fatal(err)
	}
	auth, err := userauth.New(h.DB)
	if err != nil {
		t.Fatal(err)
	}
	server := New(WithUserAuth(auth, security))
	return &authHarness{db: h, hub: security, auth: auth, handler: server.Handler(), password: password}
}

func TestBrowserAuthStatusLoginMeLogout(t *testing.T) {
	h := newAuthHarness(t)

	insecure := httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
	insecure.RemoteAddr = "10.10.0.8:43210"
	insecureRes := httptest.NewRecorder()
	h.handler.ServeHTTP(insecureRes, insecure)
	if insecureRes.Code != http.StatusUpgradeRequired {
		t.Fatalf("insecure LAN status=%d, want %d", insecureRes.Code, http.StatusUpgradeRequired)
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
	statusReq.RemoteAddr = "127.0.0.1:43210"
	statusRes := httptest.NewRecorder()
	h.handler.ServeHTTP(statusRes, statusReq)
	if statusRes.Code != http.StatusOK || !strings.Contains(statusRes.Body.String(), "SHA256:") {
		t.Fatalf("status response=%d %s", statusRes.Code, statusRes.Body.String())
	}

	wrongReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"owner","password":"wrong"}`))
	wrongReq.Header.Set("Content-Type", "application/json")
	wrongReq.RemoteAddr = "127.0.0.1:43210"
	wrongRes := httptest.NewRecorder()
	h.handler.ServeHTTP(wrongRes, wrongReq)
	if wrongRes.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password status=%d, want 401", wrongRes.Code)
	}

	loginBody, _ := json.Marshal(map[string]string{"username": "owner", "password": h.password})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.RemoteAddr = "127.0.0.1:43210"
	loginRes := httptest.NewRecorder()
	h.handler.ServeHTTP(loginRes, loginReq)
	if loginRes.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginRes.Code, loginRes.Body.String())
	}
	var loginPayload struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(loginRes.Body.Bytes(), &loginPayload); err != nil {
		t.Fatal(err)
	}
	if loginPayload.CSRFToken == "" {
		t.Fatal("login did not return CSRF token")
	}
	if strings.Contains(loginRes.Body.String(), "session_token") {
		t.Fatal("browser session token must not be returned in JSON")
	}
	cookies := loginRes.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookie count=%d, want 1", len(cookies))
	}
	sessionCookie := cookies[0]
	if sessionCookie.Name != browserSessionCookie || !sessionCookie.HttpOnly || sessionCookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("unexpected session cookie: %#v", sessionCookie)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meReq.RemoteAddr = "127.0.0.1:43210"
	meReq.AddCookie(sessionCookie)
	meRes := httptest.NewRecorder()
	h.handler.ServeHTTP(meRes, meReq)
	if meRes.Code != http.StatusOK || !strings.Contains(meRes.Body.String(), `"role":"OWNER"`) {
		t.Fatalf("me response=%d %s", meRes.Code, meRes.Body.String())
	}

	logoutWithoutCSRF := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutWithoutCSRF.RemoteAddr = "127.0.0.1:43210"
	logoutWithoutCSRF.AddCookie(sessionCookie)
	logoutWithoutCSRFRes := httptest.NewRecorder()
	h.handler.ServeHTTP(logoutWithoutCSRFRes, logoutWithoutCSRF)
	if logoutWithoutCSRFRes.Code != http.StatusForbidden {
		t.Fatalf("logout without CSRF status=%d, want 403", logoutWithoutCSRFRes.Code)
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutReq.RemoteAddr = "127.0.0.1:43210"
	logoutReq.AddCookie(sessionCookie)
	logoutReq.Header.Set(csrfHeader, loginPayload.CSRFToken)
	logoutRes := httptest.NewRecorder()
	h.handler.ServeHTTP(logoutRes, logoutReq)
	if logoutRes.Code != http.StatusNoContent {
		t.Fatalf("logout status=%d body=%s", logoutRes.Code, logoutRes.Body.String())
	}

	meAfter := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meAfter.RemoteAddr = "127.0.0.1:43210"
	meAfter.AddCookie(sessionCookie)
	meAfterRes := httptest.NewRecorder()
	h.handler.ServeHTTP(meAfterRes, meAfter)
	if meAfterRes.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session status=%d, want 401", meAfterRes.Code)
	}
}

func TestBrowserSameOriginAndRoleDenial(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()

	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	crossOrigin := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	crossOrigin.RemoteAddr = "127.0.0.1:1234"
	crossOrigin.Host = "stagecore.local"
	crossOrigin.Header.Set("Origin", "http://evil.local")
	crossOrigin.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	crossOriginRes := httptest.NewRecorder()
	h.handler.ServeHTTP(crossOriginRes, crossOrigin)
	if crossOriginRes.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status=%d, want 403", crossOriginRes.Code)
	}

	var passwordHash string
	if err := h.db.DB.QueryRowContext(ctx, `SELECT password_hash FROM local_users WHERE username = 'owner'`).Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.DB.ExecContext(ctx, `
		INSERT INTO local_users (user_id, username, password_hash, role, enabled, created_at_us, updated_at_us)
		VALUES ('00000000-0000-7000-8000-000000000002', 'viewer', ?, 'VIEWER', 1, 1, 1)
	`, passwordHash); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(ctx, "viewer", h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}

	protected := withPermission(h.auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, _ *http.Request, _ userauth.Session) {
		w.WriteHeader(http.StatusNoContent)
	})
	viewerReq := httptest.NewRequest(http.MethodGet, "/protected", nil)
	viewerReq.RemoteAddr = "127.0.0.1:1234"
	viewerReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerRes := httptest.NewRecorder()
	protected(viewerRes, viewerReq)
	if viewerRes.Code != http.StatusForbidden {
		t.Fatalf("VIEWER project.edit status=%d, want 403", viewerRes.Code)
	}

	ownerReq := httptest.NewRequest(http.MethodGet, "/protected", nil)
	ownerReq.RemoteAddr = "127.0.0.1:1234"
	ownerReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	ownerRes := httptest.NewRecorder()
	protected(ownerRes, ownerReq)
	if ownerRes.Code != http.StatusNoContent {
		t.Fatalf("OWNER project.edit status=%d, want 204", ownerRes.Code)
	}
}

func TestAuthenticatedSSEClosesAfterSessionRevocation(t *testing.T) {
	h := newAuthHarness(t)
	credential, err := h.auth.Login(context.Background(), "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(h.handler)
	defer server.Close()
	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("SSE status=%d", response.StatusCode)
	}
	reader := bufio.NewReader(response.Body)
	line, err := reader.ReadString('\n')
	if err != nil || line != "event: session\n" {
		t.Fatalf("first SSE line=%q err=%v", line, err)
	}
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatal(err)
	}

	if err := h.auth.Logout(context.Background(), credential.Token); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, reader)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SSE close read error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SSE channel remained open after session revocation")
	}
}
