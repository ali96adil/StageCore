package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

// SC-002A phase 2: explicit OWNER-only time-adjustment PREVIEW.
// This service intentionally has no OS clock setter and no APPLY route.
// A future privileged helper must be reviewed, tested and installed separately.
type hubClockPreviewRequest struct {
	Source         string `json:"source"`
	ProposedUnixMS int64  `json:"proposed_unix_ms"`
}

type hubClockPreviewResponse struct {
	PreviewOnly         bool   `json:"preview_only"`
	ApplyAvailable      bool   `json:"apply_available"`
	Source              string `json:"source"`
	ServerUTC           string `json:"server_utc"`
	ServerUnixMS        int64  `json:"server_unix_ms"`
	ProposedUTC         string `json:"proposed_utc"`
	ProposedUnixMS      int64  `json:"proposed_unix_ms"`
	DifferenceMS        int64  `json:"difference_ms"`
	ActiveSession       string `json:"active_session"`
	RuntimeBlocked      bool   `json:"runtime_blocked"`
	RequiresConfirmation bool  `json:"requires_confirmation"`
	Warning            string `json:"warning"`
}

func WithOperatorClockPreview(auth *userauth.Service, projectStore *store.Store, now func() time.Time) Option {
	return func(s *Server) {
		if auth == nil || projectStore == nil {
			return
		}
		if now == nil {
			now = time.Now
		}
		s.mux.HandleFunc("POST /api/v1/system/clock/preview", withPermission(auth, userauth.PermissionProjectRead,
			func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
				if session.User.Role != userauth.RoleOwner {
					writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "OWNER_REQUIRED"})
					return
				}
				var input hubClockPreviewRequest
				if !decodeBoundedJSON(w, r, &input) {
					return
				}
				input.Source = strings.ToUpper(strings.TrimSpace(input.Source))
				if input.Source != "PHONE" && input.Source != "MANUAL" {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "CLOCK_SOURCE_INVALID"})
					return
				}
				// Reject unrepresentable or nonsensical future operator input.
				// This is only a preview; clock bootstrap from a wrong epoch
				// is a separately audited local recovery workflow.
				proposed := time.UnixMilli(input.ProposedUnixMS).UTC()
				if proposed.Year() < 2020 || proposed.Year() > 2100 {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "CLOCK_PROPOSED_TIME_INVALID"})
					return
				}
				active, err := projectStore.ActiveOperationalSessionType(r.Context())
				if err != nil {
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "CLOCK_RUNTIME_STATUS_UNAVAILABLE"})
					return
				}
				current := now().UTC()
				w.Header().Set("Cache-Control", "no-store")
				writeJSON(w, http.StatusOK, hubClockPreviewResponse{
					PreviewOnly: true,
					ApplyAvailable: false,
					Source: input.Source,
					ServerUTC: current.Format(time.RFC3339Nano),
					ServerUnixMS: current.UnixMilli(),
					ProposedUTC: proposed.Format(time.RFC3339Nano),
					ProposedUnixMS: input.ProposedUnixMS,
					DifferenceMS: input.ProposedUnixMS - current.UnixMilli(),
					ActiveSession: string(active),
					RuntimeBlocked: active != "",
					RequiresConfirmation: true,
					Warning: "PREVIEW_ONLY_NO_CLOCK_CHANGE",
				})
			}))
	}
}
