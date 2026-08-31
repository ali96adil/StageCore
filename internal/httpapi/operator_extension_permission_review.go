package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/extension"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type operatorExtensionPermissionReview struct {
	users    *userauth.Service
	reviewer *extension.PermissionReviewer
	audit    *securityaudit.Service
}

type permissionDecisionRequest struct {
	Decision extension.PermissionDecision `json:"decision"`
}

func WithOperatorExtensionPermissionReview(users *userauth.Service, reviewer *extension.PermissionReviewer, audit *securityaudit.Service) Option {
	return func(s *Server) {
		if users == nil || reviewer == nil {
			return
		}
		h := &operatorExtensionPermissionReview{users: users, reviewer: reviewer, audit: audit}
		s.mux.HandleFunc("GET /api/v1/extensions/installations/{installation_id}/permission-review", withPermission(users, userauth.PermissionProjectRead, h.get))
		s.mux.HandleFunc("PUT /api/v1/extensions/installations/{installation_id}/permissions/{permission}", withPermission(users, userauth.PermissionPluginManage, h.decide))
	}
}

func (h *operatorExtensionPermissionReview) get(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
	review, err := h.reviewer.Get(r.Context(), strings.TrimSpace(r.PathValue("installation_id")))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "EXTENSION_INSTALLATION_NOT_FOUND"})
	case errors.Is(err, extension.ErrInstalledPayloadIntegrity), errors.Is(err, extension.ErrPermissionReviewIntegrity):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "EXTENSION_PERMISSION_REVIEW_INTEGRITY_FAILED"})
	case err != nil:
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "EXTENSION_PERMISSION_REVIEW_UNAVAILABLE"})
	default:
		writeJSON(w, http.StatusOK, review)
	}
}

func (h *operatorExtensionPermissionReview) decide(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	installationID := strings.TrimSpace(r.PathValue("installation_id"))
	permission := strings.TrimSpace(r.PathValue("permission"))
	var body permissionDecisionRequest
	if !decodeBoundedJSON(w, r, &body) {
		return
	}
	review, err := h.reviewer.Decide(r.Context(), installationID, permission, body.Decision, session.User.Username)
	if err != nil {
		status := http.StatusBadRequest
		code := "EXTENSION_PERMISSION_REVIEW_FAILED"
		switch {
		case errors.Is(err, domain.ErrShowConfigurationLocked):
			status = http.StatusConflict
			code = "SHOW_CONFIGURATION_LOCKED"
		case errors.Is(err, domain.ErrNotFound):
			status = http.StatusNotFound
			code = "EXTENSION_INSTALLATION_NOT_FOUND"
		case errors.Is(err, extension.ErrPermissionNotRequested):
			status = http.StatusBadRequest
			code = "EXTENSION_PERMISSION_NOT_REQUESTED"
		case errors.Is(err, extension.ErrInstalledPayloadIntegrity), errors.Is(err, extension.ErrPermissionReviewIntegrity):
			status = http.StatusConflict
			code = "EXTENSION_PERMISSION_REVIEW_INTEGRITY_FAILED"
		}
		h.record(r, session, installationID, permission, securityaudit.ResultRejected, code, string(body.Decision))
		writeJSON(w, status, map[string]any{"error_code": code})
		return
	}
	h.record(r, session, installationID, permission, securityaudit.ResultSuccess, "", string(body.Decision))
	writeJSON(w, http.StatusOK, review)
}

func (h *operatorExtensionPermissionReview) record(r *http.Request, session userauth.Session, installationID, permission, result, reason, decision string) {
	if h.audit == nil {
		return
	}
	_, _ = h.audit.Append(r.Context(), securityaudit.Event{
		EventType: "extension.permission.review",
		ActorUserID: session.User.ID,
		ActorUsername: session.User.Username,
		Source: "operator-web",
		ResourceType: "extension_installation",
		ResourceID: strings.TrimSpace(installationID),
		Result: result,
		Reason: reason,
		Metadata: map[string]any{
			"permission": strings.TrimSpace(permission),
			"decision": strings.ToUpper(strings.TrimSpace(decision)),
		},
	})
}
