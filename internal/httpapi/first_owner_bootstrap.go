package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/hubsecurity"
	"github.com/ali96adil/StageCore/internal/securityaudit"
)

type firstOwnerBootstrapRequest struct {
	SetupCode string `json:"setup_code"`
	Username  string `json:"username"`
	Password  string `json:"password"`
}

// WithFirstOwnerBootstrap exposes the one-time browser claim step for a fresh
// Hub. The setup code itself is still generated only through the explicit
// local setup channel; this endpoint merely consumes that code over the same
// secure/loopback browser boundary used by the authenticated Operator UI.
func WithFirstOwnerBootstrap(hub *hubsecurity.Service, audit *securityaudit.Service) Option {
	return func(s *Server) {
		if hub == nil {
			return
		}
		s.mux.HandleFunc("POST /api/v1/auth/bootstrap", func(w http.ResponseWriter, r *http.Request) {
			if !secureBrowserRequest(w, r) || !sameOriginRequest(w, r) {
				return
			}
			var body firstOwnerBootstrapRequest
			if !decodeBoundedJSON(w, r, &body) {
				return
			}
			username := strings.TrimSpace(body.Username)
			userID, err := hub.ClaimFirstOwner(r.Context(), body.SetupCode, username, body.Password)
			if err != nil {
				status := http.StatusBadRequest
				code := "BOOTSTRAP_INVALID"
				switch {
				case errors.Is(err, hubsecurity.ErrInvalidSetupCode):
					status = http.StatusUnauthorized
					code = "SETUP_CODE_INVALID"
				case errors.Is(err, hubsecurity.ErrAlreadyClaimed):
					status = http.StatusConflict
					code = "BOOTSTRAP_ALREADY_CLAIMED"
				}
				appendAudit(r, audit, securityaudit.Event{
					EventType: "hub.bootstrap.owner", ActorUsername: username, Source: loginRemoteKey(r),
					ResourceType: "hub", Result: securityaudit.ResultRejected, Reason: code,
				})
				writeJSON(w, status, map[string]any{"error_code": code})
				return
			}
			appendAudit(r, audit, securityaudit.Event{
				EventType: "hub.bootstrap.owner", ActorUserID: userID, ActorUsername: username,
				Source: loginRemoteKey(r), ResourceType: "hub", Result: securityaudit.ResultSuccess,
			})
			writeJSON(w, http.StatusCreated, map[string]any{
				"user_id": userID, "username": username, "role": hubsecurity.RoleOwner,
				"bootstrap_state": hubsecurity.BootstrapClaimed,
			})
		})
	}
}
