package httpapi

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/oscplugin"
	"github.com/ali96adil/StageCore/internal/pluginpermissions"
	"github.com/ali96adil/StageCore/internal/secretstore"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestSecurityAdministrationShowGateAndEmergencyRevocation(t *testing.T) {
	ctx := context.Background()
	h := newAuthHarness(t)
	stageStore := store.New(h.db.DB, clock.Real{})
	secrets, err := secretstore.Open(ctx, h.db.DB, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	audit, err := securityaudit.New(h.db.DB, secrets)
	if err != nil {
		t.Fatal(err)
	}
	plugins, err := pluginpermissions.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	companions := companionauth.New(stageStore, nil)
	var refreshCalls atomic.Int32
	handler := New(
		WithUserAuth(h.auth, h.hub, audit),
		WithSecurityOperations(h.auth, stageStore, secrets, plugins, audit, companions, func(context.Context) error {
			refreshCalls.Add(1)
			return nil
		}),
	).Handler()

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.CreateUser(ctx, "operator", "operator secure password", userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	operatorCredential, err := h.auth.Login(ctx, operator.Username, "operator secure password", "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}

	createSecret := securityRequest(t, handler, owner, http.MethodPost, "/api/v1/security/secrets", `{"logical_name":"projector-token","value":"alpha-secret"}`)
	if createSecret.Code != http.StatusCreated || strings.Contains(createSecret.Body.String(), "alpha-secret") {
		t.Fatalf("create secret=%d %s", createSecret.Code, createSecret.Body.String())
	}
	permissionOff := securityRequest(t, handler, owner, http.MethodPut,
		"/api/v1/security/plugins/"+oscplugin.PluginID+"/permissions/"+oscplugin.PermissionUDPSend,
		`{"granted":false}`,
	)
	if permissionOff.Code != http.StatusOK || refreshCalls.Load() != 1 {
		t.Fatalf("permission revoke=%d body=%s refresh=%d", permissionOff.Code, permissionOff.Body.String(), refreshCalls.Load())
	}
	permissionOn := securityRequest(t, handler, owner, http.MethodPut,
		"/api/v1/security/plugins/"+oscplugin.PluginID+"/permissions/"+oscplugin.PermissionUDPSend,
		`{"granted":true}`,
	)
	if permissionOn.Code != http.StatusOK || refreshCalls.Load() != 2 {
		t.Fatalf("permission grant=%d body=%s refresh=%d", permissionOn.Code, permissionOn.Body.String(), refreshCalls.Load())
	}

	companionID := pairSecurityTestCompanion(t, ctx, companions)
	startShowSession(t, ctx, stageStore)

	blockedSecret := securityRequest(t, handler, owner, http.MethodPut, "/api/v1/security/secrets/projector-token", `{"value":"beta-secret"}`)
	if blockedSecret.Code != http.StatusConflict || !strings.Contains(blockedSecret.Body.String(), "SHOW_ADMINISTRATION_BLOCKED") {
		t.Fatalf("SHOW secret mutation=%d %s", blockedSecret.Code, blockedSecret.Body.String())
	}
	value, err := secrets.Resolve(ctx, secretstore.Reference("projector-token"))
	if err != nil || value != "alpha-secret" {
		t.Fatalf("blocked secret changed value=%q err=%v", value, err)
	}
	blockedPlugin := securityRequest(t, handler, owner, http.MethodPut,
		"/api/v1/security/plugins/"+oscplugin.PluginID+"/permissions/"+oscplugin.PermissionUDPSend,
		`{"granted":false}`,
	)
	if blockedPlugin.Code != http.StatusConflict || refreshCalls.Load() != 2 {
		t.Fatalf("SHOW plugin mutation=%d refresh=%d", blockedPlugin.Code, refreshCalls.Load())
	}
	blockedUser := securityRequest(t, handler, owner, http.MethodPost, "/api/v1/security/users", `{"username":"during-show","password":"during show secure password","role":"VIEWER"}`)
	if blockedUser.Code != http.StatusConflict {
		t.Fatalf("SHOW user mutation=%d %s", blockedUser.Code, blockedUser.Body.String())
	}

	revokeUser := securityRequest(t, handler, owner, http.MethodPost,
		"/api/v1/security/users/"+operator.ID+"/revoke-sessions",
		`{"reason":"suspected compromise","confirm":"REVOKE"}`,
	)
	if revokeUser.Code != http.StatusNoContent {
		t.Fatalf("emergency user revoke=%d %s", revokeUser.Code, revokeUser.Body.String())
	}
	if _, err := h.auth.Validate(ctx, operatorCredential.Token); !errors.Is(err, userauth.ErrSessionInvalid) {
		t.Fatalf("revoked operator session remained valid: %v", err)
	}

	revokeCompanion := securityRequest(t, handler, owner, http.MethodPost,
		"/api/v1/security/companions/"+companionID+"/revoke",
		`{"reason":"lost device","confirm":"REVOKE"}`,
	)
	if revokeCompanion.Code != http.StatusNoContent {
		t.Fatalf("emergency Companion revoke=%d %s", revokeCompanion.Code, revokeCompanion.Body.String())
	}
	companion, err := stageStore.GetCompanion(ctx, companionID)
	if err != nil || companion.TrustState != domain.CompanionRevoked {
		t.Fatalf("Companion trust=%#v err=%v", companion, err)
	}

	records, err := audit.List(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	assertAuditEvent(t, records, "secret.update", securityaudit.ResultRejected, "SHOW_ADMINISTRATION_BLOCKED")
	assertAuditEvent(t, records, "plugin.permission.change", securityaudit.ResultRejected, "SHOW_ADMINISTRATION_BLOCKED")
	assertAuditEvent(t, records, "user.sessions.revoke", securityaudit.ResultSuccess, "suspected compromise")
	assertAuditEvent(t, records, "companion.revoke", securityaudit.ResultSuccess, "lost device")
}

func securityRequest(t *testing.T, handler http.Handler, credential userauth.Credential, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.RemoteAddr = "127.0.0.1:45678"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(csrfHeader, credential.CSRFToken)
	request.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func startShowSession(t *testing.T, ctx context.Context, stageStore *store.Store) {
	t.Helper()
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Security SHOW", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if runtimeSnapshot.ProjectID != project.ID {
		t.Fatal("Snapshot Project mismatch")
	}
	if _, err := stageStore.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "security gate test"); err != nil {
		t.Fatal(err)
	}
}

func pairSecurityTestCompanion(t *testing.T, ctx context.Context, service *companionauth.Service) string {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicBytes := elliptic.Marshal(elliptic.P256(), privateKey.X, privateKey.Y)
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	companionID := "44444444-4444-4444-8444-444444444444"
	receipt, err := service.RequestPairing(ctx, companionauth.PairingRequestInput{
		CompanionID: companionID, DisplayName: "Security Mac", Hostname: "security.local",
		Platform: "macos", Architecture: "arm64", Version: "0.1.0", Capabilities: []string{"local.echo"},
		PublicKeyAlgorithm: domain.CompanionPublicKeyAlgorithm,
		PublicKeyBase64: base64.StdEncoding.EncodeToString(publicBytes),
		ClientNonceBase64: base64.StdEncoding.EncodeToString(nonce),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApprovePairing(ctx, receipt.RequestID, receipt.PairingCode, companionauth.Approval{Actor: "owner", Authorized: true}); err != nil {
		t.Fatal(err)
	}
	return companionID
}

func assertAuditEvent(t *testing.T, records []securityaudit.Record, eventType, result, reason string) {
	t.Helper()
	for _, record := range records {
		if record.EventType == eventType && record.Result == result && record.Reason == reason {
			return
		}
	}
	encoded, _ := json.Marshal(records)
	t.Fatalf("audit event %s/%s/%s not found: %s", eventType, result, reason, encoded)
}
