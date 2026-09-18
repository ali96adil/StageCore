package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type lightingConfigurationRequest struct {
	Configuration lightingnode.Configuration `json:"configuration"`
	Aliases       map[string]string          `json:"aliases"`
}

type lightingHealthView struct {
	Status              string                        `json:"status"`
	Reasons             []string                      `json:"reasons"`
	Connection          deviceexperience.ConnectionState `json:"connection"`
	Readiness           deviceexperience.Readiness       `json:"readiness"`
	LastSeenAt          *time.Time                     `json:"last_seen_at,omitempty"`
	DMXHealthy          *bool                          `json:"dmx_healthy,omitempty"`
	BrownoutWarning     bool                           `json:"brownout_warning"`
	Authority           lightingnode.AuthoritySource   `json:"authority,omitempty"`
	ExpectedConfigHash  string                         `json:"expected_configuration_hash,omitempty"`
	ObservedConfigHash  string                         `json:"observed_configuration_hash,omitempty"`
	ConfigurationMatches *bool                         `json:"configuration_matches,omitempty"`
}

type lightingConfigurationNodeView struct {
	DeviceID       string                     `json:"device_id"`
	DisplayName    string                     `json:"display_name"`
	Enabled        bool                       `json:"enabled"`
	Configured     bool                       `json:"configured"`
	Configuration *lightingnode.Configuration `json:"configuration,omitempty"`
	Aliases        map[string]string          `json:"aliases,omitempty"`
	Observed       *lightingnode.Observation  `json:"observed,omitempty"`
	Health         lightingHealthView         `json:"health"`
}

func registerOperatorLightingConfigurationRoutes(
	mux *http.ServeMux,
	auth *userauth.Service,
	devices *deviceexperience.Repository,
	stageStore *store.Store,
) {
	mux.HandleFunc("GET /api/v1/projects/{project_id}/lighting-controller/configuration", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		projectID := strings.TrimSpace(r.PathValue("project_id"))
		project, err := stageStore.GetProject(r.Context(), projectID)
		if err != nil {
			writeProjectStoreError(w, err)
			return
		}
		revision, err := stageStore.GetRevision(r.Context(), project.CurrentRevisionID)
		if err != nil {
			writeProjectStoreError(w, err)
			return
		}
		bindings, err := stageStore.ListLightingNodeBindings(r.Context(), revision.ID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "LIGHTING_BINDINGS_UNAVAILABLE", "detail": err.Error()})
			return
		}
		allDevices, err := devices.ListDevices(r.Context(), projectID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "LIGHTING_DEVICES_UNAVAILABLE", "detail": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"revision": makeRevisionView(revision),
			"nodes":    lightingConfigurationNodes(bindings, allDevices),
		})
	}))

	mux.HandleFunc("PUT /api/v1/projects/{project_id}/lighting-controller/configuration/{device_id}", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		projectID := strings.TrimSpace(r.PathValue("project_id"))
		deviceID := strings.TrimSpace(r.PathValue("device_id"))
		if err := stageStore.RequireProjectConfigurationMutable(r.Context(), projectID); err != nil {
			if errors.Is(err, domain.ErrShowConfigurationLocked) {
				writeJSON(w, http.StatusLocked, map[string]any{"error": "SHOW_CONFIGURATION_LOCKED", "detail": err.Error()})
				return
			}
			writeProjectStoreError(w, err)
			return
		}
		device, err := devices.GetDevice(r.Context(), deviceID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "LIGHTING_DEVICE_NOT_FOUND"})
			return
		}
		if device.ProjectID != projectID || device.ProfileID != lightingnode.ProfileID {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "LIGHTING_DEVICE_INVALID"})
			return
		}
		var input lightingConfigurationRequest
		if !decodeBoundedJSON(w, r, &input) {
			return
		}
		revision, err := stageStore.EnsureProjectDraft(r.Context(), projectID, session.User.ID, "Lighting node configuration edit")
		if err != nil {
			writeProjectStoreError(w, err)
			return
		}
		binding, err := stageStore.SetLightingNodeBinding(r.Context(), revision.ID, lightingnode.ProjectBinding{
			DeviceID: device.ID, ProfileID: lightingnode.ProfileID,
			Configuration: input.Configuration, Aliases: input.Aliases,
		}, session.User.ID)
		if err != nil {
			switch {
			case errors.Is(err, domain.ErrInvalidInput):
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "LIGHTING_CONFIGURATION_INVALID", "detail": err.Error()})
			case errors.Is(err, domain.ErrConflict):
				writeJSON(w, http.StatusConflict, map[string]any{"error": "LIGHTING_CONFIGURATION_CONFLICT", "detail": err.Error()})
			case errors.Is(err, domain.ErrShowConfigurationLocked), errors.Is(err, domain.ErrRevisionFrozen):
				writeJSON(w, http.StatusLocked, map[string]any{"error": "SHOW_CONFIGURATION_LOCKED", "detail": err.Error()})
			default:
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "LIGHTING_CONFIGURATION_SAVE_FAILED", "detail": err.Error()})
			}
			return
		}
		hash, _ := lightingnode.ConfigurationHash(binding.Configuration)
		writeJSON(w, http.StatusOK, map[string]any{
			"revision":                    makeRevisionView(revision),
			"binding":                     binding,
			"expected_configuration_hash": hash,
		})
	}))

	mux.HandleFunc("DELETE /api/v1/projects/{project_id}/lighting-controller/configuration/{device_id}", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		projectID := strings.TrimSpace(r.PathValue("project_id"))
		deviceID := strings.TrimSpace(r.PathValue("device_id"))
		if strings.ToLower(strings.TrimSpace(r.URL.Query().Get("confirm"))) != "true" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "CONFIRMATION_REQUIRED"})
			return
		}
		if err := stageStore.RequireProjectConfigurationMutable(r.Context(), projectID); err != nil {
			if errors.Is(err, domain.ErrShowConfigurationLocked) {
				writeJSON(w, http.StatusLocked, map[string]any{"error": "SHOW_CONFIGURATION_LOCKED", "detail": err.Error()})
				return
			}
			writeProjectStoreError(w, err)
			return
		}
		project, err := stageStore.GetProject(r.Context(), projectID)
		if err != nil {
			writeProjectStoreError(w, err)
			return
		}
		device, err := devices.GetDevice(r.Context(), deviceID)
		if err != nil || device.ProjectID != projectID || device.ProfileID != lightingnode.ProfileID {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "LIGHTING_DEVICE_NOT_FOUND"})
			return
		}
		revision, err := stageStore.EnsureProjectDraft(r.Context(), project.ID, session.User.ID, "Lighting node configuration removal")
		if err != nil {
			writeProjectStoreError(w, err)
			return
		}
		if err := stageStore.DeleteLightingNodeBinding(r.Context(), revision.ID, deviceID); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "LIGHTING_CONFIGURATION_NOT_FOUND"})
				return
			}
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "LIGHTING_CONFIGURATION_DELETE_FAILED", "detail": err.Error()})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

func lightingConfigurationNodes(bindings []lightingnode.ProjectBinding, all []deviceexperience.Device) []lightingConfigurationNodeView {
	bindingByDevice := make(map[string]lightingnode.ProjectBinding, len(bindings))
	for _, binding := range bindings {
		bindingByDevice[binding.DeviceID] = binding
	}
	out := make([]lightingConfigurationNodeView, 0)
	for _, device := range all {
		if device.ProfileID != lightingnode.ProfileID || device.ProtocolVersion != deviceexperience.ProtocolVersion1 {
			continue
		}
		binding, configured := bindingByDevice[device.ID]
		var bindingPtr *lightingnode.ProjectBinding
		if configured {
			copyBinding := binding
			bindingPtr = &copyBinding
		}
		health, observed := assessLightingHealth(device, bindingPtr)
		view := lightingConfigurationNodeView{
			DeviceID: device.ID, DisplayName: device.DisplayName, Enabled: device.Enabled,
			Configured: configured, Health: health, Observed: observed,
		}
		if configured {
			config := binding.Configuration
			config.Channels = append([]lightingnode.ChannelConfig(nil), binding.Configuration.Channels...)
			view.Configuration = &config
			view.Aliases = make(map[string]string, len(binding.Aliases))
			for alias, channelKey := range binding.Aliases {
				view.Aliases[alias] = channelKey
			}
		}
		out = append(out, view)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DisplayName == out[j].DisplayName {
			return out[i].DeviceID < out[j].DeviceID
		}
		return out[i].DisplayName < out[j].DisplayName
	})
	return out
}

func assessLightingHealth(device deviceexperience.Device, binding *lightingnode.ProjectBinding) (lightingHealthView, *lightingnode.Observation) {
	view := lightingHealthView{
		Status: "READY", Reasons: make([]string, 0),
		Connection: deviceexperience.ConnectionOffline,
		Readiness: deviceexperience.ReadinessUnknown,
	}
	severity := 0
	add := func(level int, reason string) {
		if level > severity {
			severity = level
		}
		for _, existing := range view.Reasons {
			if existing == reason {
				return
			}
		}
		view.Reasons = append(view.Reasons, reason)
	}

	if !device.Enabled {
		add(2, "DEVICE_DISABLED")
	}
	if binding == nil {
		add(2, "CONFIGURATION_REQUIRED")
	} else {
		hash, err := lightingnode.ConfigurationHash(binding.Configuration)
		if err != nil {
			add(2, "CONFIGURATION_INVALID")
		} else {
			view.ExpectedConfigHash = hash
		}
	}

	if device.Runtime == nil {
		add(2, "DEVICE_NEVER_OBSERVED")
		view.Status = lightingHealthStatus(severity)
		return view, nil
	}
	view.Connection = device.Runtime.Connection
	view.Readiness = device.Runtime.Readiness
	lastSeen := device.Runtime.LastSeenAt
	view.LastSeenAt = &lastSeen

	switch device.Runtime.Connection {
	case deviceexperience.ConnectionOnline:
	case deviceexperience.ConnectionStale:
		add(2, "DEVICE_STALE")
	case deviceexperience.ConnectionRevoked:
		add(2, "DEVICE_REVOKED")
	default:
		add(2, "DEVICE_OFFLINE")
	}
	switch device.Runtime.Readiness {
	case deviceexperience.ReadinessReady:
	case deviceexperience.ReadinessBlocker:
		add(2, "DEVICE_READINESS_BLOCKER")
	case deviceexperience.ReadinessWarning:
		add(1, "DEVICE_READINESS_WARNING")
	case deviceexperience.ReadinessAdvisory:
		add(1, "DEVICE_READINESS_ADVISORY")
	default:
		add(1, "DEVICE_READINESS_UNKNOWN")
	}

	var observed lightingnode.Observation
	if len(device.Runtime.ObservedState) == 0 || json.Unmarshal(device.Runtime.ObservedState, &observed) != nil || observed.SchemaVersion != lightingnode.SchemaVersion1 {
		add(1, "OBSERVATION_INVALID")
		view.Status = lightingHealthStatus(severity)
		return view, nil
	}
	dmxHealthy := observed.DMXHealthy
	view.DMXHealthy = &dmxHealthy
	view.BrownoutWarning = observed.BrownoutWarning
	view.Authority = observed.Authority
	view.ObservedConfigHash = strings.TrimSpace(observed.ConfigurationHash)

	if !observed.DMXHealthy {
		add(2, "DMX_UNHEALTHY")
	}
	if observed.BrownoutWarning {
		add(1, "BROWNOUT_WARNING")
	}
	switch observed.Authority {
	case lightingnode.AuthorityStageCore:
	case lightingnode.AuthorityFailsafe:
		add(2, "FAILSAFE_AUTHORITY_ACTIVE")
	case lightingnode.AuthorityLocalWeb:
		add(1, "LOCAL_WEB_AUTHORITY_ACTIVE")
	default:
		add(1, "AUTHORITY_UNKNOWN")
	}
	if view.ExpectedConfigHash != "" {
		if view.ObservedConfigHash == "" {
			add(1, "CONFIGURATION_HASH_UNKNOWN")
		} else {
			matches := view.ExpectedConfigHash == view.ObservedConfigHash
			view.ConfigurationMatches = &matches
			if !matches {
				add(1, "CONFIGURATION_NOT_APPLIED")
			}
		}
	}
	view.Status = lightingHealthStatus(severity)
	return view, &observed
}

func lightingHealthStatus(severity int) string {
	switch severity {
	case 0:
		return "READY"
	case 1:
		return "WARNING"
	default:
		return "BLOCKER"
	}
}
