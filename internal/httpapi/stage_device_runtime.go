package httpapi

import (
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/devicechannel"
)

// WithStageDeviceRuntime exposes the authenticated Stage Device WebSocket on
// the secure device gateway. Stage Devices deliberately reuse the hardened
// Companion pairing/session authority while speaking a separate runtime
// protocol and command contract.
func WithStageDeviceRuntime(auth *companionauth.Service, runtime *devicechannel.Runtime) Option {
	return func(s *Server) {
		if s == nil || s.mux == nil || auth == nil || runtime == nil {
			return
		}
		s.mux.HandleFunc("GET /api/v1/stage-devices/runtime", func(w http.ResponseWriter, r *http.Request) {
			if !secureDeviceRequest(r) {
				writeJSON(w, http.StatusUpgradeRequired, map[string]any{"error_code": "SECURE_TRANSPORT_REQUIRED"})
				return
			}
			const prefix = "StageCoreSession "
			authorization := r.Header.Get("Authorization")
			if !strings.HasPrefix(authorization, prefix) || strings.TrimSpace(strings.TrimPrefix(authorization, prefix)) == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error_code": companionauth.CodeSessionInvalid})
				return
			}
			token := strings.TrimSpace(strings.TrimPrefix(authorization, prefix))
			session, err := auth.ValidateRuntimeSession(r.Context(), token)
			if err != nil {
				writeCompanionAuthError(w, err)
				return
			}
			runtime.ServeWebSocket(w, r, session, token)
		})
	}
}
