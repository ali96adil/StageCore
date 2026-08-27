package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/oscplugin"
	"github.com/ali96adil/StageCore/internal/pluginpermissions"
	"github.com/ali96adil/StageCore/internal/secretstore"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type PluginPermissionRefresher func(context.Context) error

type securityOperations struct {
	users         *userauth.Service
	store         *store.Store
	secrets       *secretstore.Service
	plugins       *pluginpermissions.Service
	audit         *securityaudit.Service
	companions    *companionauth.Service
	refreshPlugin PluginPermissionRefresher
}

func WithSecurityOperations(
	users *userauth.Service,
	stageStore *store.Store,
	secrets *secretstore.Service,
	plugins *pluginpermissions.Service,
	audit *securityaudit.Service,
	companions *companionauth.Service,
	refresh PluginPermissionRefresher,
) Option {
	return func(s *Server) {
		if users == nil || stageStore == nil || secrets == nil || plugins == nil || audit == nil || companions == nil {
			return
		}
		ops := &securityOperations{
			users: users, store: stageStore, secrets: secrets, plugins: plugins,
			audit: audit, companions: companions, refreshPlugin: refresh,
		}
		ops.register(s.mux)
	}
}

func (s *securityOperations) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/security/audit", withPermission(s.users, userauth.PermissionAuditRead, s.handleAudit))

	mux.HandleFunc("GET /api/v1/security/secrets", withPermission(s.users, userauth.PermissionSecretManage, s.handleSecretsList))
	mux.HandleFunc("POST /api/v1/security/secrets", withPermission(s.users, userauth.PermissionSecretManage, s.handleSecretCreate))
	mux.HandleFunc("PUT /api/v1/security/secrets/{logical_name}", withPermission(s.users, userauth.PermissionSecretManage, s.handleSecretUpdate))
	mux.HandleFunc("DELETE /api/v1/security/secrets/{logical_name}", withPermission(s.users, userauth.PermissionSecretManage, s.handleSecretDelete))

	mux.HandleFunc("GET /api/v1/security/plugins/permissions", withPermission(s.users, userauth.PermissionPluginManage, s.handlePluginPermissions))
	mux.HandleFunc("PUT /api/v1/security/plugins/{plugin_id}/permissions/{permission}", withPermission(s.users, userauth.PermissionPluginManage, s.handlePluginPermissionSet))

	mux.HandleFunc("GET /api/v1/security/users", withPermission(s.users, userauth.PermissionUserManage, s.handleUsersList))
	mux.HandleFunc("POST /api/v1/security/users", withPermission(s.users, userauth.PermissionUserManage, s.handleUserCreate))
	mux.HandleFunc("PUT /api/v1/security/users/{user_id}/role", withPermission(s.users, userauth.PermissionUserManage, s.handleUserRole))
	mux.HandleFunc("PUT /api/v1/security/users/{user_id}/enabled", withPermission(s.users, userauth.PermissionUserManage, s.handleUserEnabled))
	mux.HandleFunc("POST /api/v1/security/users/{user_id}/revoke-sessions", withPermission(s.users, userauth.PermissionUserManage, s.handleUserSessionRevoke))

	mux.HandleFunc("POST /api/v1/security/companions/pairing/approve", withPermission(s.users, userauth.PermissionCompanionPair, s.handleCompanionPairingApprove))
	mux.HandleFunc("POST /api/v1/security/companions/{companion_id}/revoke", withPermission(s.users, userauth.PermissionCompanionRevoke, s.handleCompanionRevoke))
}

func (s *securityOperations) handleAudit(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
	limit := 100
	if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			limit = parsed
		}
	}
	records, err := s.audit.List(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "AUDIT_UNAVAILABLE"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": records})
}

func (s *securityOperations) handleSecretsList(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
	items, err := s.secrets.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "SECRET_STORE_UNAVAILABLE"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"secrets": items})
}

type secretCreateRequest struct {
	LogicalName string `json:"logical_name"`
	Value       string `json:"value"`
}

type secretUpdateRequest struct {
	Value string `json:"value"`
}

func (s *securityOperations) handleSecretCreate(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	if s.rejectShowAdmin(w, r, session, "secret.create", "secret", "") {
		return
	}
	var body secretCreateRequest
	if !decodeBoundedJSON(w, r, &body) {
		return
	}
	metadata, err := s.secrets.Create(r.Context(), body.LogicalName, body.Value)
	if err != nil {
		s.record(r, session, "secret.create", "secret", strings.TrimSpace(body.LogicalName), securityaudit.ResultFailed, "SECRET_CREATE_FAILED", nil)
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "SECRET_CREATE_FAILED"})
		return
	}
	s.record(r, session, "secret.create", "secret", metadata.Reference, securityaudit.ResultSuccess, "", map[string]any{"logical_name": metadata.LogicalName})
	writeJSON(w, http.StatusCreated, metadata)
}

func (s *securityOperations) handleSecretUpdate(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	logicalName := strings.TrimSpace(r.PathValue("logical_name"))
	reference := secretstore.Reference(logicalName)
	if s.rejectShowAdmin(w, r, session, "secret.update", "secret", reference) {
		return
	}
	var body secretUpdateRequest
	if !decodeBoundedJSON(w, r, &body) {
		return
	}
	metadata, err := s.secrets.Update(r.Context(), reference, body.Value)
	if err != nil {
		s.record(r, session, "secret.update", "secret", reference, securityaudit.ResultFailed, "SECRET_UPDATE_FAILED", nil)
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "SECRET_UPDATE_FAILED"})
		return
	}
	s.record(r, session, "secret.update", "secret", metadata.Reference, securityaudit.ResultSuccess, "", nil)
	writeJSON(w, http.StatusOK, metadata)
}

func (s *securityOperations) handleSecretDelete(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	reference := secretstore.Reference(strings.TrimSpace(r.PathValue("logical_name")))
	if s.rejectShowAdmin(w, r, session, "secret.delete", "secret", reference) {
		return
	}
	if err := s.secrets.Delete(r.Context(), reference); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, secretstore.ErrNotFound) {
			status = http.StatusNotFound
		}
		s.record(r, session, "secret.delete", "secret", reference, securityaudit.ResultFailed, "SECRET_DELETE_FAILED", nil)
		writeJSON(w, status, map[string]any{"error_code": "SECRET_DELETE_FAILED"})
		return
	}
	s.record(r, session, "secret.delete", "secret", reference, securityaudit.ResultSuccess, "", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *securityOperations) handlePluginPermissions(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
	grants, err := s.plugins.List(r.Context(), strings.TrimSpace(r.URL.Query().Get("plugin_id")))
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "PLUGIN_PERMISSIONS_UNAVAILABLE"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"permissions": grants})
}

type pluginPermissionRequest struct {
	Granted bool `json:"granted"`
}

func (s *securityOperations) handlePluginPermissionSet(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	pluginID := strings.TrimSpace(r.PathValue("plugin_id"))
	permission := strings.TrimSpace(r.PathValue("permission"))
	resourceID := pluginID + ":" + permission
	if pluginID != oscplugin.PluginID || (permission != oscplugin.PermissionUDPSend && permission != oscplugin.PermissionUDPListen) {
		s.record(r, session, "plugin.permission.change", "plugin_permission", resourceID, securityaudit.ResultRejected, "UNKNOWN_FIRST_PARTY_PERMISSION", nil)
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "PLUGIN_PERMISSION_UNKNOWN"})
		return
	}
	if s.rejectShowAdmin(w, r, session, "plugin.permission.change", "plugin_permission", resourceID) {
		return
	}
	var body pluginPermissionRequest
	if !decodeBoundedJSON(w, r, &body) {
		return
	}
	grant, err := s.plugins.Set(r.Context(), pluginID, permission, body.Granted, session.User.Username)
	if err != nil {
		s.record(r, session, "plugin.permission.change", "plugin_permission", resourceID, securityaudit.ResultFailed, "PLUGIN_PERMISSION_WRITE_FAILED", nil)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "PLUGIN_PERMISSION_WRITE_FAILED"})
		return
	}
	if s.refreshPlugin != nil {
		if err := s.refreshPlugin(r.Context()); err != nil {
			s.record(r, session, "plugin.permission.change", "plugin_permission", resourceID, securityaudit.ResultFailed, "PLUGIN_PERMISSION_APPLY_FAILED", nil)
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "PLUGIN_PERMISSION_APPLY_FAILED"})
			return
		}
	}
	s.record(r, session, "plugin.permission.change", "plugin_permission", resourceID, securityaudit.ResultSuccess, "", map[string]any{"granted": body.Granted})
	writeJSON(w, http.StatusOK, grant)
}

func (s *securityOperations) handleUsersList(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
	users, err := s.users.ListUsers(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "USER_ADMIN_UNAVAILABLE"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

type userCreateRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type userRoleRequest struct { Role string `json:"role"` }
type userEnabledRequest struct { Enabled bool `json:"enabled"` }
type revokeRequest struct {
	Reason  string `json:"reason"`
	Confirm string `json:"confirm"`
}

func (s *securityOperations) handleUserCreate(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	if s.rejectShowAdmin(w, r, session, "user.create", "user", "") {
		return
	}
	var body userCreateRequest
	if !decodeBoundedJSON(w, r, &body) {
		return
	}
	user, err := s.users.CreateUser(r.Context(), body.Username, body.Password, body.Role)
	if err != nil {
		s.record(r, session, "user.create", "user", strings.TrimSpace(body.Username), securityaudit.ResultFailed, "USER_CREATE_FAILED", map[string]any{"role": body.Role})
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "USER_CREATE_FAILED"})
		return
	}
	s.record(r, session, "user.create", "user", user.ID, securityaudit.ResultSuccess, "", map[string]any{"username": user.Username, "role": user.Role})
	writeJSON(w, http.StatusCreated, user)
}

func (s *securityOperations) handleUserRole(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	userID := strings.TrimSpace(r.PathValue("user_id"))
	if s.rejectShowAdmin(w, r, session, "user.role.change", "user", userID) {
		return
	}
	var body userRoleRequest
	if !decodeBoundedJSON(w, r, &body) {
		return
	}
	user, err := s.users.SetUserRole(r.Context(), userID, body.Role)
	if err != nil {
		code := "USER_ROLE_CHANGE_FAILED"
		if errors.Is(err, userauth.ErrLastOwner) {
			code = "LAST_OWNER_REQUIRED"
		}
		s.record(r, session, "user.role.change", "user", userID, securityaudit.ResultRejected, code, map[string]any{"role": body.Role})
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": code})
		return
	}
	s.record(r, session, "user.role.change", "user", user.ID, securityaudit.ResultSuccess, "", map[string]any{"role": user.Role})
	writeJSON(w, http.StatusOK, user)
}

func (s *securityOperations) handleUserEnabled(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	userID := strings.TrimSpace(r.PathValue("user_id"))
	if s.rejectShowAdmin(w, r, session, "user.enabled.change", "user", userID) {
		return
	}
	var body userEnabledRequest
	if !decodeBoundedJSON(w, r, &body) {
		return
	}
	user, err := s.users.SetUserEnabled(r.Context(), userID, body.Enabled)
	if err != nil {
		code := "USER_ENABLED_CHANGE_FAILED"
		if errors.Is(err, userauth.ErrLastOwner) {
			code = "LAST_OWNER_REQUIRED"
		}
		s.record(r, session, "user.enabled.change", "user", userID, securityaudit.ResultRejected, code, map[string]any{"enabled": body.Enabled})
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": code})
		return
	}
	s.record(r, session, "user.enabled.change", "user", user.ID, securityaudit.ResultSuccess, "", map[string]any{"enabled": user.Enabled})
	writeJSON(w, http.StatusOK, user)
}

func (s *securityOperations) handleUserSessionRevoke(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	userID := strings.TrimSpace(r.PathValue("user_id"))
	var body revokeRequest
	if !decodeBoundedJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Confirm) != "REVOKE" || strings.TrimSpace(body.Reason) == "" {
		s.record(r, session, "user.sessions.revoke", "user", userID, securityaudit.ResultRejected, "STRONG_CONFIRMATION_REQUIRED", nil)
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "STRONG_CONFIRMATION_REQUIRED"})
		return
	}
	if err := s.users.RevokeUserSessions(r.Context(), userID); err != nil {
		s.record(r, session, "user.sessions.revoke", "user", userID, securityaudit.ResultFailed, "SESSION_REVOCATION_FAILED", nil)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "SESSION_REVOCATION_FAILED"})
		return
	}
	s.record(r, session, "user.sessions.revoke", "user", userID, securityaudit.ResultSuccess, strings.TrimSpace(body.Reason), map[string]any{"emergency": s.showActive(r.Context())})
	w.WriteHeader(http.StatusNoContent)
}

type companionPairingApproveRequest struct {
	RequestID   string `json:"request_id"`
	PairingCode string `json:"pairing_code"`
}

func (s *securityOperations) handleCompanionPairingApprove(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	if s.rejectShowAdmin(w, r, session, "companion.pairing.approve", "companion", "") {
		return
	}
	var body companionPairingApproveRequest
	if !decodeBoundedJSON(w, r, &body) {
		return
	}
	key, err := s.companions.ApprovePairing(r.Context(), body.RequestID, body.PairingCode, companionauth.Approval{Actor: session.User.Username, Authorized: true})
	if err != nil {
		s.record(r, session, "companion.pairing.approve", "companion", "", securityaudit.ResultRejected, string(companionauth.ErrorCode(err)), map[string]any{"request_id": body.RequestID})
		writeCompanionAuthError(w, err)
		return
	}
	s.record(r, session, "companion.pairing.approve", "companion", key.CompanionID, securityaudit.ResultSuccess, "", map[string]any{"public_key_fingerprint": key.PublicKeyFingerprint})
	writeJSON(w, http.StatusOK, map[string]any{"companion_id": key.CompanionID, "public_key_fingerprint": key.PublicKeyFingerprint})
}

func (s *securityOperations) handleCompanionRevoke(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	companionID := strings.TrimSpace(r.PathValue("companion_id"))
	var body revokeRequest
	if !decodeBoundedJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Confirm) != "REVOKE" || strings.TrimSpace(body.Reason) == "" {
		s.record(r, session, "companion.revoke", "companion", companionID, securityaudit.ResultRejected, "STRONG_CONFIRMATION_REQUIRED", nil)
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "STRONG_CONFIRMATION_REQUIRED"})
		return
	}
	if err := s.companions.Revoke(r.Context(), companionID, session.User.Username, body.Reason, true); err != nil {
		s.record(r, session, "companion.revoke", "companion", companionID, securityaudit.ResultFailed, "COMPANION_REVOCATION_FAILED", nil)
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "COMPANION_REVOCATION_FAILED"})
		return
	}
	s.record(r, session, "companion.revoke", "companion", companionID, securityaudit.ResultSuccess, strings.TrimSpace(body.Reason), map[string]any{"emergency": s.showActive(r.Context())})
	w.WriteHeader(http.StatusNoContent)
}

func (s *securityOperations) rejectShowAdmin(w http.ResponseWriter, r *http.Request, session userauth.Session, eventType, resourceType, resourceID string) bool {
	activeType, err := s.store.ActiveOperationalSessionType(r.Context())
	if err != nil {
		s.record(r, session, eventType, resourceType, resourceID, securityaudit.ResultFailed, "SHOW_STATE_UNAVAILABLE", nil)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "SHOW_STATE_UNAVAILABLE"})
		return true
	}
	if activeType != domain.SessionShow {
		return false
	}
	s.record(r, session, eventType, resourceType, resourceID, securityaudit.ResultRejected, "SHOW_ADMINISTRATION_BLOCKED", map[string]any{"session_type": activeType})
	writeJSON(w, http.StatusConflict, map[string]any{"error_code": "SHOW_ADMINISTRATION_BLOCKED"})
	return true
}

func (s *securityOperations) showActive(ctx context.Context) bool {
	activeType, err := s.store.ActiveOperationalSessionType(ctx)
	return err == nil && activeType == domain.SessionShow
}

func (s *securityOperations) record(r *http.Request, session userauth.Session, eventType, resourceType, resourceID, result, reason string, metadata any) {
	_, _ = s.audit.Append(r.Context(), securityaudit.Event{
		EventType: eventType, ActorUserID: session.User.ID, ActorUsername: session.User.Username,
		Source: loginRemoteKey(r), ResourceType: resourceType, ResourceID: resourceID,
		Result: result, Reason: reason, Metadata: metadata,
	})
}
