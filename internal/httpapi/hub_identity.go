package httpapi

import (
	"context"
	"net/http"

	"github.com/ali96adil/StageCore/internal/hubsecurity"
)

type hubIdentityProvider interface {
	Identity(context.Context) (hubsecurity.Identity, error)
}

// WithHubIdentity adds the deliberately public identity probe used after
// F-004 TLS pinning. It carries no credential or secret material.
func WithHubIdentity(provider hubIdentityProvider) Option {
	return func(s *Server) {
		if provider == nil {
			return
		}
		s.mux.HandleFunc("GET /api/v1/hub/identity", func(w http.ResponseWriter, r *http.Request) {
			identity, err := provider.Identity(r.Context())
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "HUB_IDENTITY_UNAVAILABLE"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"schema_version":  1,
				"hub_id":          identity.HubID,
				"display_name":    identity.DisplayName,
				"fingerprint":     identity.Fingerprint,
				"bootstrap_state": identity.BootstrapState,
			})
		})
	}
}
