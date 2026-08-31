package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/deviceprofile"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type deviceProfileMatchRequest struct {
	Attributes map[string]string `json:"attributes"`
}

type deviceProfileMaterializeRequest struct {
	Values map[string]any `json:"values"`
}

func WithOperatorDeviceProfiles(auth *userauth.Service, catalog *deviceprofile.Catalog) Option {
	return func(s *Server) {
		if auth == nil || catalog == nil {
			return
		}
		registerOperatorDeviceProfileRoutes(s.mux, auth, catalog)
	}
}

func registerOperatorDeviceProfileRoutes(mux *http.ServeMux, auth *userauth.Service, catalog *deviceprofile.Catalog) {
	mux.HandleFunc("GET /api/v1/device-profiles", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, _ *http.Request, _ userauth.Session) {
		writeJSON(w, http.StatusOK, map[string]any{
			"schema_version": catalog.SchemaVersion(),
			"profiles":       catalog.List(),
		})
	}))

	mux.HandleFunc("GET /api/v1/device-profiles/{profile_id}", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		profile, err := catalog.Get(strings.TrimSpace(r.PathValue("profile_id")))
		if errors.Is(err, deviceprofile.ErrProfileNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "DEVICE_PROFILE_NOT_FOUND"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "DEVICE_PROFILE_CATALOG_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"profile": profile})
	}))

	mux.HandleFunc("POST /api/v1/device-profiles/match", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		var body deviceProfileMatchRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		if len(body.Attributes) > 64 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "DEVICE_OBSERVATION_INVALID"})
			return
		}
		for key, value := range body.Attributes {
			if len(strings.TrimSpace(key)) == 0 || len(key) > 128 || len(value) > 2048 {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "DEVICE_OBSERVATION_INVALID"})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"matches": catalog.Match(deviceprofile.Observation{Attributes: body.Attributes}),
		})
	}))

	mux.HandleFunc("POST /api/v1/device-profiles/{profile_id}/materialize", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		var body deviceProfileMaterializeRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		if len(body.Values) > 64 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "DEVICE_PROFILE_VALUES_INVALID"})
			return
		}
		result, err := catalog.Materialize(strings.TrimSpace(r.PathValue("profile_id")), body.Values)
		if errors.Is(err, deviceprofile.ErrProfileNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "DEVICE_PROFILE_NOT_FOUND"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error_code": "DEVICE_PROFILE_VALUES_INVALID",
				"message":    err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"target": result})
	}))
}
