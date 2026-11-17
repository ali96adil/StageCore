package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/oscplugin"
	"github.com/ali96adil/StageCore/internal/pluginpermissions"
	"github.com/ali96adil/StageCore/internal/secretstore"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestSecurityOperationsWithholdResultsAfterOwnerSessionRevocation(t *testing.T) {
	for _, route := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "read plugin grants", method: http.MethodGet, path: "/api/v1/security/plugins/permissions"},
		{
			name: "change plugin grant", method: http.MethodPut,
			path: "/api/v1/security/plugins/" + oscplugin.PluginID + "/permissions/" + oscplugin.PermissionUDPSend,
			body: `{"granted":true}`,
		},
	} {
		t.Run(route.name, func(t *testing.T) {
			h := newAuthHarness(t)
			ctx := context.Background()
			var ownerToken string
			_, plugins, handler := newSecurityOperationsRevocationFixture(t, h, func() {
				if err := h.auth.Logout(ctx, ownerToken); err != nil {
					t.Errorf("revoke Owner session: %v", err)
				}
			}, nil)
			owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
			if err != nil {
				t.Fatal(err)
			}
			ownerToken = owner.Token
			response := securityRequest(t, handler, owner, route.method, route.path, route.body)
			if _, err := h.auth.Validate(ctx, owner.Token); err == nil {
				t.Fatal("test did not revoke the Owner session before delivery")
			}
			if response.Code != http.StatusForbidden {
				t.Fatalf("revoked Owner received security-operation result: HTTP %d body=%s", response.Code, response.Body.String())
			}
			var payload map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if route.method == http.MethodPut {
				if payload["error_code"] != "SECURITY_OPERATION_AUTHORITY_REVOKED" ||
					payload["effect_may_have_been_committed"] != true || payload["retryable"] != false {
					t.Fatalf("committed plugin permission outcome was not fenced truthfully: %+v", payload)
				}
				granted, err := plugins.Granted(ctx, oscplugin.PluginID)
				if err != nil || !containsString(granted, oscplugin.PermissionUDPSend) {
					t.Fatalf("plugin grant write was rolled back or lost: grants=%v err=%v", granted, err)
				}
			} else if _, leaked := payload["permissions"]; leaked {
				t.Fatalf("revoked Owner received permission inventory: %+v", payload)
			}
		})
	}
}

func TestSecurityOperationDoesNotCreateSecretAfterRevocationWhileReadingBody(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	_, _, handler := newSecurityOperationsRevocationFixture(t, h, nil, nil)
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	request := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/security/secrets",
		[]byte(`{"logical_name":"revoked-secret","value":"private-value"}`), owner.Token, owner.CSRFToken)
	request.Body = &firmwareUploadRevokeReader{ReadCloser: request.Body, revoke: func() {
		if err := h.auth.Logout(ctx, owner.Token); err != nil {
			t.Errorf("revoke Owner while request body is read: %v", err)
		}
	}}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("revoked Owner response=%d want=%d body=%s", response.Code, http.StatusForbidden, response.Body.String())
	}
	var denied map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &denied); err != nil {
		t.Fatal(err)
	}
	if denied["effect_may_have_been_committed"] != false || denied["retryable"] != false || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("pre-dispatch denial response overstated or cached the result: headers=%v body=%+v", response.Header(), denied)
	}
	var count int
	if err := h.db.DB.QueryRowContext(ctx, `SELECT count(*) FROM secret_records WHERE logical_name = 'revoked-secret'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("secret mutation was committed after the Owner session was revoked while reading the body")
	}
}

func TestSecurityOperationRechecksShowGateAfterReadingPluginPermissionBody(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	_, plugins, handler := newSecurityOperationsRevocationFixture(t, h, nil, nil)
	stageStore := store.New(h.db.DB, clock.Real{})
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plugins.Set(ctx, oscplugin.PluginID, oscplugin.PermissionUDPSend, false, owner.Session.User.Username); err != nil {
		t.Fatalf("disable UDP send before SHOW-gate test: %v", err)
	}
	initial, err := plugins.Granted(ctx, oscplugin.PluginID)
	if err != nil || containsString(initial, oscplugin.PermissionUDPSend) {
		t.Fatalf("expected UDP send to start disabled for the SHOW-gate test: grants=%v err=%v", initial, err)
	}
	request := authenticatedMutationRequest(t, http.MethodPut,
		"/api/v1/security/plugins/"+oscplugin.PluginID+"/permissions/"+oscplugin.PermissionUDPSend,
		[]byte(`{"granted":true}`), owner.Token, owner.CSRFToken)
	request.Body = &firmwareUploadRevokeReader{ReadCloser: request.Body, revoke: func() {
		startShowSession(t, ctx, stageStore)
	}}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "SHOW_ADMINISTRATION_BLOCKED") {
		t.Fatalf("plugin grant during SHOW transition response=%d body=%s", response.Code, response.Body.String())
	}
	grants, err := plugins.Granted(ctx, oscplugin.PluginID)
	if err != nil {
		t.Fatal(err)
	}
	if containsString(grants, oscplugin.PermissionUDPSend) {
		t.Fatal("plugin network permission changed after SHOW started while the request body was read")
	}
}

func TestSecurityPluginApplyFailureReportsCommittedEffect(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	_, plugins, handler := newSecurityOperationsRevocationFixture(t, h, nil, errors.New("refresh failed"))
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	response := securityRequest(t, handler, owner, http.MethodPut,
		"/api/v1/security/plugins/"+oscplugin.PluginID+"/permissions/"+oscplugin.PermissionUDPSend,
		`{"granted":true}`)
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusServiceUnavailable || payload["effect_may_have_been_committed"] != true ||
		payload["retryable"] != false || strings.Contains(response.Body.String(), "permission") {
		t.Fatalf("refresh failure response hid the persisted grant outcome incorrectly: HTTP %d body=%s", response.Code, response.Body.String())
	}
	granted, err := plugins.Granted(ctx, oscplugin.PluginID)
	if err != nil || !containsString(granted, oscplugin.PermissionUDPSend) {
		t.Fatalf("plugin grant write was rolled back or lost: grants=%v err=%v", granted, err)
	}
}

func newSecurityOperationsRevocationFixture(
	t *testing.T,
	h *authHarness,
	beforeResponse func(),
	refreshError error,
) (*securityaudit.Service, *pluginpermissions.Service, http.Handler) {
	t.Helper()
	ctx := context.Background()
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
	refresh := func(context.Context) error { return refreshError }
	return audit, plugins, New(withSecurityOperations(h.auth, stageStore, secrets, plugins, audit, companions, refresh,
		securityOperationsHooks{BeforeResponse: beforeResponse})).Handler()
}
