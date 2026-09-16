package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/ali96adil/StageCore/internal/haauthority"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type haAuthorityControl interface {
	Status(context.Context) (haauthority.SupervisorStatus, error)
	Activate(context.Context) error
	Release(context.Context) error
	Demote()
}

type operatorHAAuthority struct {
	users     *userauth.Service
	authority haAuthorityControl
	audit     *securityaudit.Service
}

// WithOperatorHAAuthority registers HA control only when WITNESS mode has
// provided a supervised authority surface. STANDALONE therefore exposes no
// promotion endpoint at all.
func WithOperatorHAAuthority(users *userauth.Service, authority haAuthorityControl, audit *securityaudit.Service) Option {
	return func(s *Server) {
		if users == nil || authority == nil {
			return
		}
		h := &operatorHAAuthority{users: users, authority: authority, audit: audit}
		s.mux.HandleFunc("GET /api/v1/ha/authority", withPermission(users, userauth.PermissionHAManage, h.status))
		s.mux.HandleFunc("POST /api/v1/ha/authority/activate", withPermission(users, userauth.PermissionHAManage, h.activate))
		s.mux.HandleFunc("POST /api/v1/ha/authority/release", withPermission(users, userauth.PermissionHAManage, h.release))
		s.mux.HandleFunc("POST /api/v1/ha/authority/demote", withPermission(users, userauth.PermissionHAManage, h.demote))
	}
}

func (h *operatorHAAuthority) status(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
	status, err := h.authority.Status(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "HA_AUTHORITY_STATUS_UNAVAILABLE"})
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *operatorHAAuthority) activate(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	err := h.authority.Activate(r.Context())
	if err != nil {
		status := http.StatusServiceUnavailable
		code := "HA_AUTHORITY_ACTIVATION_FAILED"
		switch {
		case errors.Is(err, haauthority.ErrOperationalSessionActive):
			status = http.StatusConflict
			code = "HA_AUTHORITY_ACTIVATION_BLOCKED_ACTIVE_SESSION"
		case errors.Is(err, haauthority.ErrSupervisorClosed):
			code = "HA_AUTHORITY_SUPERVISOR_CLOSED"
		}
		h.record(r, session, "ha.authority.activate", securityaudit.ResultRejected, code)
		writeJSON(w, status, map[string]any{"error_code": code})
		return
	}
	status, statusErr := h.authority.Status(r.Context())
	if statusErr != nil {
		h.record(r, session, "ha.authority.activate", securityaudit.ResultSuccess, "")
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "LEADER"})
		return
	}
	h.record(r, session, "ha.authority.activate", securityaudit.ResultSuccess, "")
	writeJSON(w, http.StatusOK, status)
}

func (h *operatorHAAuthority) release(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	err := h.authority.Release(r.Context())
	if err != nil {
		status := http.StatusServiceUnavailable
		code := "HA_AUTHORITY_RELEASE_FAILED"
		if errors.Is(err, haauthority.ErrNoActiveLease) {
			status = http.StatusConflict
			code = "HA_AUTHORITY_NOT_ACTIVE"
		}
		h.record(r, session, "ha.authority.release", securityaudit.ResultRejected, code)
		writeJSON(w, status, map[string]any{"error_code": code})
		return
	}
	status, statusErr := h.authority.Status(r.Context())
	if statusErr != nil {
		h.record(r, session, "ha.authority.release", securityaudit.ResultSuccess, "")
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "STANDBY"})
		return
	}
	h.record(r, session, "ha.authority.release", securityaudit.ResultSuccess, "")
	writeJSON(w, http.StatusOK, status)
}

func (h *operatorHAAuthority) demote(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	h.authority.Demote()
	status, err := h.authority.Status(r.Context())
	if err != nil {
		h.record(r, session, "ha.authority.demote", securityaudit.ResultSuccess, "")
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "STANDBY"})
		return
	}
	h.record(r, session, "ha.authority.demote", securityaudit.ResultSuccess, "")
	writeJSON(w, http.StatusOK, status)
}

func (h *operatorHAAuthority) record(r *http.Request, session userauth.Session, eventType, result, reason string) {
	if h.audit == nil {
		return
	}
	status, _ := h.authority.Status(r.Context())
	_, _ = h.audit.Append(r.Context(), securityaudit.Event{
		EventType:     eventType,
		ActorUserID:   session.User.ID,
		ActorUsername: session.User.Username,
		Source:        "operator-web",
		ResourceType:  "ha_authority",
		ResourceID:    "local-hub",
		Result:        result,
		Reason:        reason,
		Metadata: map[string]any{
			"mode":           status.Mode,
			"holder_id":      status.HolderID,
			"epoch":          status.Epoch,
			"remaining_ms":   status.RemainingMS,
			"renewal_active": status.RenewalActive,
		},
	})
}
