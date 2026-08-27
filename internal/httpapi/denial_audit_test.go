package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestAuthenticatedPermissionDenialIsAudited(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	if _, err := h.auth.CreateUser(ctx, "viewer-audit", "viewer audit password", userauth.RoleViewer); err != nil {
		t.Fatal(err)
	}
	credential, err := h.auth.Login(ctx, "viewer-audit", "viewer audit password", "127.0.0.3")
	if err != nil {
		t.Fatal(err)
	}
	audit, err := securityaudit.New(h.db.DB, nil)
	if err != nil {
		t.Fatal(err)
	}
	protected := withPermission(h.auth, userauth.PermissionSecretManage, func(w http.ResponseWriter, _ *http.Request, _ userauth.Session) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := AuditDeniedRequests(protected, h.auth, audit)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/security/secrets", nil)
	req.RemoteAddr = "127.0.0.1:17020"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", res.Code)
	}
	records, err := audit.List(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("audit record count=%d, want 1", len(records))
	}
	record := records[0]
	if record.EventType != "authorization.denied" || record.Result != securityaudit.ResultRejected || record.ActorUsername != "viewer-audit" {
		t.Fatalf("audit record=%#v", record)
	}
	if record.ResourceID != "GET /api/v1/security/secrets" {
		t.Fatalf("resource=%q", record.ResourceID)
	}
}
