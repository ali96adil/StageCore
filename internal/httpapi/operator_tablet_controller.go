package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

var tabletAliasSlugRE = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

type tabletObservedScope struct {
	ProjectID         string `json:"project_id"`
	RuntimeSnapshotID string `json:"runtime_snapshot_id"`
	TabletManifestID  string `json:"tablet_manifest_id,omitempty"`
}

type tabletCommandRequest struct {
	All           bool            `json:"all"`
	GroupName     string          `json:"group_name"`
	DeviceIDs     []string        `json:"device_ids"`
	CommandType   string          `json:"command_type"`
	SessionID     string          `json:"session_id"`
	CorrelationID string          `json:"correlation_id"`
	Priority      string          `json:"priority"`
	Confirm       string          `json:"confirm"`
	Payload       json.RawMessage `json:"payload"`
}

type tabletCueActionRequest struct {
	DeviceIDs     []string        `json:"device_ids"`
	CommandType   string          `json:"command_type"`
	ExecutionMode string          `json:"execution_mode"`
	Priority      string          `json:"priority"`
	Payload       json.RawMessage `json:"payload"`
}

type tabletCueActionDescriptor struct {
	DeviceID      string          `json:"device_id"`
	DisplayName   string          `json:"display_name"`
	TargetRef     string          `json:"target_ref"`
	CapabilityKey string          `json:"capability_key"`
	ExecutionMode string          `json:"execution_mode"`
	Priority      string          `json:"priority"`
	Parameters    json.RawMessage `json:"parameters"`
}

// WithOperatorTabletController provides the tablet-specific Operator facade.
// It deliberately reuses the canonical Stage Device runtime and Cue Engine
// targets; no second transport, command queue, or playback authority is added.
func WithOperatorTabletController(
	auth *userauth.Service,
	devices *deviceexperience.Repository,
	runtime *devicechannel.Runtime,
	stageStore *store.Store,
) Option {
	return func(s *Server) {
		if s == nil || s.mux == nil || auth == nil || devices == nil || runtime == nil || stageStore == nil {
			return
		}

		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/tablet-controller", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			all, err := devices.ListDevices(r.Context(), projectID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "TABLET_LIST_FAILED", "detail": err.Error()})
				return
			}
			tablets := tabletDevices(all)
			groups := make([]string, 0)
			seenGroups := map[string]bool{}
			for _, tablet := range tablets {
				group := strings.TrimSpace(tablet.GroupName)
				if group != "" && !seenGroups[group] {
					seenGroups[group] = true
					groups = append(groups, group)
				}
			}
			sort.Strings(groups)
			writeJSON(w, http.StatusOK, map[string]any{
				"devices": tablets,
				"groups":  groups,
			})
		}))

		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/tablet-controller/devices/{device_id}/assign", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			if userauth.Authorize(session.User.Role, userauth.PermissionCompanionPair) != nil {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "TABLET_ASSIGNMENT_PERMISSION_REQUIRED"})
				return
			}
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			deviceID := strings.TrimSpace(r.PathValue("device_id"))
			if projectID == "" || deviceID == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "TABLET_ASSIGNMENT_SCOPE_REQUIRED"})
				return
			}
			if err := stageStore.RequireProjectConfigurationMutable(r.Context(), projectID); err != nil {
				writeJSON(w, http.StatusLocked, map[string]any{"error": "SHOW_CONFIGURATION_LOCKED", "detail": err.Error()})
				return
			}
			var input struct {
				ExpectedProjectID        string `json:"expected_project_id"`
				ExpectedRuntimeSnapshotID string `json:"expected_runtime_snapshot_id"`
				ExpectedEpoch            int64  `json:"expected_assignment_epoch"`
				RuntimeSnapshotID         string `json:"runtime_snapshot_id"`
			}
			if !decodeBoundedJSON(w, r, &input) {
				return
			}
			input.RuntimeSnapshotID = strings.TrimSpace(input.RuntimeSnapshotID)
			snapshot, err := stageStore.GetRuntimeSnapshot(r.Context(), input.RuntimeSnapshotID)
			if err != nil || snapshot.ProjectID != projectID || snapshot.Status != domain.SnapshotPublished {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "TABLET_ASSIGNMENT_SNAPSHOT_INVALID"})
				return
			}
			intent := deviceexperience.TabletAssignmentInput{
				DeviceID: deviceID,
				ExpectedProjectID: input.ExpectedProjectID,
				ExpectedRuntimeSnapshotID: input.ExpectedRuntimeSnapshotID,
				TargetProjectID: projectID,
				TargetRuntimeSnapshotID: input.RuntimeSnapshotID,
				ExpectedEpoch: input.ExpectedEpoch,
			}
			if _, err := devices.PreflightTabletAssignment(r.Context(), intent); err != nil {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error": "TABLET_ASSIGNMENT_PREFLIGHT_BLOCKED",
					"detail": err.Error(),
				})
				return
			}
			token, ok := browserSessionToken(r)
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "AUTH_REQUIRED"})
				return
			}
			csrf := r.Header.Get(csrfHeader)
			actor := session.User.ID
			reauthorize := func(ctx context.Context) error {
				next, err := auth.ValidateCSRF(ctx, token, csrf)
				if err != nil {
					return err
				}
				if next.User.ID != actor {
					return userauth.ErrForbidden
				}
				if err := userauth.Authorize(next.User.Role, userauth.PermissionProjectEdit); err != nil {
					return err
				}
				return userauth.Authorize(next.User.Role, userauth.PermissionCompanionPair)
			}
			record, err := runtime.ExecuteTabletAssignmentAuthorized(r.Context(), intent, actor, reauthorize)
			if err != nil {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error": "TABLET_ASSIGNMENT_NOT_COMMITTED",
					"detail": err.Error(),
					"note": "REFETCH_HUB_ASSIGNMENT_BEFORE_RETRY",
				})
				return
			}
			writeJSON(w, http.StatusAccepted, map[string]any{
				"assignment": record,
				"commands_enabled": false,
				"reconnect_required": true,
				"safe_media_acknowledged": true,
			})
		}))

		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/tablet-controller/live-flash", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			if _, err := stageStore.GetProject(r.Context(), projectID); err != nil {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "PROJECT_NOT_FOUND"})
				return
			}
			var input struct {
				URL   string `json:"url"`
				State string `json:"state"`
			}
			if !decodeBoundedJSON(w, r, &input) {
				return
			}
			state := strings.ToLower(strings.TrimSpace(input.State))
			if state != "auto" && state != "on" && state != "off" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "TABLET_LIVE_FLASH_STATE_INVALID"})
				return
			}
			controlURL, err := tabletRelayFlashControlURL(input.URL, state)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "TABLET_LIVE_FLASH_URL_INVALID", "detail": err.Error()})
				return
			}
			request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, controlURL, nil)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "TABLET_LIVE_FLASH_REQUEST_INVALID"})
				return
			}
			client := &http.Client{
				Timeout: 3 * time.Second,
				Transport: &http.Transport{Proxy: nil},
				CheckRedirect: func(*http.Request, []*http.Request) error {
					return fmt.Errorf("relay redirects are not allowed")
				},
			}
			response, err := client.Do(request)
			if err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]any{"error": "TABLET_LIVE_FLASH_RELAY_UNAVAILABLE", "detail": err.Error()})
				return
			}
			defer response.Body.Close()
			body, err := io.ReadAll(io.LimitReader(response.Body, 8<<10))
			if err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]any{"error": "TABLET_LIVE_FLASH_RELAY_RESPONSE_INVALID"})
				return
			}
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				writeJSON(w, http.StatusBadGateway, map[string]any{
					"error": "TABLET_LIVE_FLASH_RELAY_REJECTED",
					"relay_status": response.StatusCode,
					"detail": strings.TrimSpace(string(body)),
				})
				return
			}
			var relayState map[string]any
			if len(body) != 0 && json.Unmarshal(body, &relayState) != nil {
				writeJSON(w, http.StatusBadGateway, map[string]any{"error": "TABLET_LIVE_FLASH_RELAY_RESPONSE_INVALID"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"state": state, "relay": relayState})
		}))

		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/tablet-controller/commands", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			var input tabletCommandRequest
			if !decodeBoundedJSON(w, r, &input) {
				return
			}
			input.CommandType = strings.TrimSpace(input.CommandType)
			capability := deviceexperience.RequiredCapability(input.CommandType)
			if capability == "" || !tabletRuntimeCapability(capability) {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "TABLET_COMMAND_UNSUPPORTED"})
				return
			}
			normalizedPayload, payloadErr := normalizeTabletCommandPayload(input.CommandType, input.Payload)
			if payloadErr != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "TABLET_COMMAND_PAYLOAD_INVALID", "detail": payloadErr.Error()})
				return
			}
			all, err := devices.ListDevices(r.Context(), projectID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "TABLET_LIST_FAILED", "detail": err.Error()})
				return
			}
			targets, ok := selectTabletTargets(tabletDevices(all), input.All, input.GroupName, input.DeviceIDs)
			if !ok {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "TABLET_TARGET_INVALID", "detail": "choose exactly one of all, group_name or device_ids"})
				return
			}
			if len(targets) == 0 {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "TABLET_TARGET_EMPTY"})
				return
			}
			settingsCommand := tabletSettingsCommand(input.CommandType)
			if settingsCommand {
				for _, tablet := range targets {
					if tablet.Runtime == nil || tablet.Runtime.Connection != deviceexperience.ConnectionOnline ||
						tablet.Runtime.Readiness != deviceexperience.ReadinessReady {
						writeJSON(w, http.StatusConflict, map[string]any{
							"error": "TABLET_SETTINGS_DEVICE_NOT_READY",
							"device_id": tablet.ID,
						})
						return
					}
				}
				if len(targets) > 1 {
					active, activeErr := stageStore.ActiveSessionForProject(r.Context(), projectID)
					if activeErr != nil {
						writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "TABLET_SETTINGS_SHOW_CHECK_FAILED"})
						return
					}
					if active != nil && active.Type == domain.SessionShow &&
						strings.TrimSpace(input.Confirm) != "APPLY_TABLET_SETTINGS_DURING_SHOW" {
						writeJSON(w, http.StatusConflict, map[string]any{"error": "TABLET_SETTINGS_SHOW_CONFIRMATION_REQUIRED"})
						return
					}
				}
			}
			correlationID := strings.TrimSpace(input.CorrelationID)
			if correlationID == "" {
				correlationID, err = stageid.New()
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "CORRELATION_ID_FAILED"})
					return
				}
			}
			type result struct {
				DeviceID    string                          `json:"device_id"`
				DisplayName string                          `json:"display_name"`
				Command     *deviceexperience.DeviceCommand `json:"command,omitempty"`
				Error       string                          `json:"error,omitempty"`
			}
			results := make([]result, 0, len(targets))
			for _, tablet := range targets {
				if !containsString(tablet.Capabilities, capability) {
					results = append(results, result{DeviceID: tablet.ID, DisplayName: tablet.DisplayName, Error: "CAPABILITY_UNAVAILABLE"})
					continue
				}
				scope, scopeErr := tabletScope(tablet, projectID)
				if scopeErr != nil {
					results = append(results, result{DeviceID: tablet.ID, DisplayName: tablet.DisplayName, Error: scopeErr.Error()})
					continue
				}
				payload := mergeTabletManifestScope(normalizedPayload, scope.TabletManifestID)
				command, dispatchErr := runtime.Dispatch(r.Context(), deviceexperience.CreateCommandInput{
					ProjectID:         projectID,
					SessionID:         strings.TrimSpace(input.SessionID),
					DeviceID:          tablet.ID,
					CommandType:       input.CommandType,
					Issuer:            session.User.ID,
					CorrelationID:     correlationID,
					RuntimeSnapshotID: scope.RuntimeSnapshotID,
					Priority:          defaultTabletPriority(input.Priority),
					IdempotencyKey:    correlationID + ":" + input.CommandType + ":" + tablet.ID,
					Payload:           payload,
				})
				if dispatchErr != nil {
					results = append(results, result{DeviceID: tablet.ID, DisplayName: tablet.DisplayName, Error: dispatchErr.Error()})
					continue
				}
				copy := command
				results = append(results, result{DeviceID: tablet.ID, DisplayName: tablet.DisplayName, Command: &copy})
			}
			writeJSON(w, http.StatusOK, map[string]any{"correlation_id": correlationID, "results": results})
		}))

		// Read back the durable terminal state of a Tablet Controller command.
		// Dispatch is asynchronous, so the initial POST may legitimately return
		// ACCEPTED before the physical tablet reports COMPLETED/FAILED/REJECTED.
		// This endpoint lets Operator surface the real tablet result instead of
		// treating transport acceptance as execution success.
		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/tablet-controller/commands/{command_id}", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			commandID := strings.TrimSpace(r.PathValue("command_id"))
			if projectID == "" || commandID == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "TABLET_COMMAND_SCOPE_REQUIRED"})
				return
			}
			command, err := devices.GetCommand(r.Context(), commandID)
			if err != nil || command.Envelope.ProjectID != projectID {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "TABLET_COMMAND_NOT_FOUND"})
				return
			}
			device, err := devices.GetDevice(r.Context(), command.DeviceID)
			if err != nil || device.Kind != deviceexperience.DeviceTabletPlayer {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "TABLET_COMMAND_NOT_FOUND"})
				return
			}
			writeJSON(w, http.StatusOK, command)
		}))

		// Convert a visual Tablet Action into canonical Cue Engine actions. This
		// endpoint owns the Stage Device alias detail so the Operator UI never
		// needs to expose target_ref, capability strings, or JSON configuration.
		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/tablet-controller/cue-actions", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			if err := stageStore.RequireProjectConfigurationMutable(r.Context(), projectID); err != nil {
				writeJSON(w, http.StatusLocked, map[string]any{"error": "SHOW_CONFIGURATION_LOCKED", "detail": err.Error()})
				return
			}
			var input tabletCueActionRequest
			if !decodeBoundedJSON(w, r, &input) {
				return
			}
			input.CommandType = strings.TrimSpace(input.CommandType)
			capability := deviceexperience.RequiredCapability(input.CommandType)
			if capability == "" || !strings.HasPrefix(capability, "tablet.media.") {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "TABLET_COMMAND_UNSUPPORTED"})
				return
			}
			normalizedPayload, payloadErr := normalizeTabletCommandPayload(input.CommandType, input.Payload)
			if payloadErr != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "TABLET_COMMAND_PAYLOAD_INVALID", "detail": payloadErr.Error()})
				return
			}
			if len(input.DeviceIDs) == 0 {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "TABLET_TARGET_EMPTY"})
				return
			}
			all, err := devices.ListDevices(r.Context(), projectID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "TABLET_LIST_FAILED", "detail": err.Error()})
				return
			}
			targets, ok := selectTabletTargets(tabletDevices(all), false, "", input.DeviceIDs)
			if !ok || len(targets) != len(uniqueStrings(input.DeviceIDs)) {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "TABLET_TARGET_INVALID"})
				return
			}
			aliases, err := stageStore.ListAliases(r.Context(), projectID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "TABLET_ALIAS_LIST_FAILED", "detail": err.Error()})
				return
			}
			actions := make([]tabletCueActionDescriptor, 0, len(targets))
			for _, tablet := range targets {
				if !containsString(tablet.Capabilities, capability) {
					writeJSON(w, http.StatusConflict, map[string]any{"error": "TABLET_CAPABILITY_UNAVAILABLE", "device_id": tablet.ID, "capability": capability})
					return
				}
				targetRef, ensureErr := ensureTabletAlias(r, stageStore, aliases, projectID, tablet, capability)
				if ensureErr != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "TABLET_ALIAS_FAILED", "detail": ensureErr.Error()})
					return
				}
				actions = append(actions, tabletCueActionDescriptor{
					DeviceID:      tablet.ID,
					DisplayName:   tablet.DisplayName,
					TargetRef:     targetRef,
					CapabilityKey: capability,
					ExecutionMode: defaultTabletExecutionMode(input.ExecutionMode),
					Priority:      defaultTabletPriority(input.Priority),
					Parameters:    normalizedPayload,
				})
			}
			writeJSON(w, http.StatusOK, map[string]any{"actions": actions})
		}))
	}
}

func tabletDevices(all []deviceexperience.Device) []deviceexperience.Device {
	out := make([]deviceexperience.Device, 0)
	for _, device := range all {
		if !device.Enabled || device.Kind != deviceexperience.DeviceTabletPlayer {
			continue
		}
		if device.ProtocolVersion == deviceexperience.ProtocolVersion1 {
			out = append(out, device)
			continue
		}
		if device.ProtocolVersion == deviceexperience.ProtocolVersion2 &&
			device.Assignment != nil && device.Assignment.State == "ACTIVE" &&
			device.Assignment.ProjectID != "" && device.Assignment.RuntimeSnapshotID != "" {
			out = append(out, device)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DisplayName == out[j].DisplayName {
			return out[i].ID < out[j].ID
		}
		return out[i].DisplayName < out[j].DisplayName
	})
	return out
}

func selectTabletTargets(all []deviceexperience.Device, selectAll bool, group string, ids []string) ([]deviceexperience.Device, bool) {
	group = strings.TrimSpace(group)
	wanted := uniqueStrings(ids)
	selectors := 0
	if selectAll { selectors++ }
	if group != "" { selectors++ }
	if len(wanted) > 0 { selectors++ }
	if selectors != 1 { return nil, false }
	wantedSet := map[string]bool{}
	for _, id := range wanted { wantedSet[id] = true }
	out := make([]deviceexperience.Device, 0)
	for _, tablet := range all {
		if !tablet.Enabled { continue }
		if selectAll || (group != "" && tablet.GroupName == group) || wantedSet[tablet.ID] {
			out = append(out, tablet)
		}
	}
	return out, true
}

func tabletScope(device deviceexperience.Device, projectID string) (tabletObservedScope, error) {
	if device.Runtime == nil || device.Runtime.Connection != deviceexperience.ConnectionOnline {
		return tabletObservedScope{}, fmt.Errorf("DEVICE_OFFLINE")
	}
	if device.ProtocolVersion == deviceexperience.ProtocolVersion2 {
		if device.Assignment == nil || device.Assignment.State != "ACTIVE" ||
			device.Assignment.ProjectID != projectID ||
			strings.TrimSpace(device.Assignment.RuntimeSnapshotID) == "" {
			return tabletObservedScope{}, fmt.Errorf("HUB_ASSIGNMENT_SCOPE_MISMATCH")
		}
		scope := tabletObservedScope{
			ProjectID: projectID,
			RuntimeSnapshotID: device.Assignment.RuntimeSnapshotID,
		}
		// Tablet Manifest ID remains an optional content hint. It may be read
		// from live observation, but it never grants Project/Snapshot authority.
		if len(device.Runtime.ObservedState) != 0 {
			var observed tabletObservedScope
			if json.Unmarshal(device.Runtime.ObservedState, &observed) == nil {
				scope.TabletManifestID = strings.TrimSpace(observed.TabletManifestID)
			}
		}
		return scope, nil
	}
	var scope tabletObservedScope
	if len(device.Runtime.ObservedState) == 0 || json.Unmarshal(device.Runtime.ObservedState, &scope) != nil {
		return tabletObservedScope{}, fmt.Errorf("RUNTIME_SCOPE_UNAVAILABLE")
	}
	if strings.TrimSpace(scope.ProjectID) == "" || scope.ProjectID != projectID {
		return tabletObservedScope{}, fmt.Errorf("PROJECT_SCOPE_MISMATCH")
	}
	if strings.TrimSpace(scope.RuntimeSnapshotID) == "" {
		return tabletObservedScope{}, fmt.Errorf("SNAPSHOT_SCOPE_UNAVAILABLE")
	}
	return scope, nil
}

func mergeTabletManifestScope(payload json.RawMessage, manifestID string) json.RawMessage {
	var object map[string]any
	if len(payload) == 0 || json.Unmarshal(payload, &object) != nil || object == nil {
		object = map[string]any{}
	}
	if strings.TrimSpace(manifestID) != "" {
		object["tablet_manifest_id"] = manifestID
	}
	encoded, err := json.Marshal(object)
	if err != nil { return json.RawMessage(`{}`) }
	return encoded
}

func ensureTabletAlias(r *http.Request, stageStore *store.Store, aliases []domain.ProjectDeviceAlias, projectID string, tablet deviceexperience.Device, capability string) (string, error) {
	for _, alias := range aliases {
		if alias.LogicalType != devicechannel.StageDeviceLogicalType { continue }
		var cfg struct {
			DeviceID      string `json:"device_id"`
			CapabilityKey string `json:"capability_key"`
		}
		if json.Unmarshal(alias.ProjectConfig, &cfg) == nil && cfg.DeviceID == tablet.ID && cfg.CapabilityKey == capability {
			return alias.LogicalName, nil
		}
	}
	config, err := json.Marshal(map[string]string{"device_id": tablet.ID, "capability_key": capability})
	if err != nil { return "", err }
	logicalName := "tablet." + tabletAliasSlug(tablet.ID) + "." + tabletAliasSlug(strings.TrimPrefix(capability, "tablet.media."))
	created, err := stageStore.CreateAlias(r.Context(), domain.ProjectDeviceAlias{
		ProjectID:     projectID,
		LogicalName:   logicalName,
		LogicalType:   devicechannel.StageDeviceLogicalType,
		TargetRef:     tablet.ID,
		GroupName:     tablet.GroupName,
		ProjectConfig: config,
	})
	if err != nil { return "", err }
	return created.LogicalName, nil
}

func tabletAliasSlug(value string) string {
	value = tabletAliasSlugRE.ReplaceAllString(strings.TrimSpace(value), "-")
	value = strings.Trim(value, "-._")
	if value == "" { return "device" }
	return value
}

func defaultTabletExecutionMode(value string) string {
	switch strings.TrimSpace(value) {
	case "SEQUENTIAL", "PARALLEL", "PARALLEL_BARRIER":
		return strings.TrimSpace(value)
	default:
		return "PARALLEL_BARRIER"
	}
}

func defaultTabletPriority(value string) string {
	switch strings.TrimSpace(value) {
	case "P0", "P1", "P2", "P3":
		return strings.TrimSpace(value)
	default:
		return "P1"
	}
}

func tabletRelayFlashControlURL(rawLiveURL, state string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawLiveURL))
	if err != nil || parsed == nil || !strings.EqualFold(parsed.Scheme, "http") ||
		parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", fmt.Errorf("relay Live URL must be absolute HTTP without credentials or fragment")
	}
	if parsed.Path != "/api/v0/stream" {
		return "", fmt.Errorf("relay Live URL path must be /api/v0/stream")
	}
	if parsed.Port() != "9081" {
		return "", fmt.Errorf("relay Live URL must use port 9081")
	}
	ip := net.ParseIP(parsed.Hostname())
	if ip == nil || (!ip.IsPrivate() && !ip.IsLoopback()) {
		return "", fmt.Errorf("relay Live URL host must be a private or loopback IP")
	}
	for key := range parsed.Query() {
		if key != "flash" {
			return "", fmt.Errorf("relay Live URL contains unsupported query field %q", key)
		}
	}
	control := &url.URL{Scheme: "http", Host: parsed.Host, Path: "/api/v0/flash"}
	query := control.Query()
	query.Set("state", strings.ToLower(strings.TrimSpace(state)))
	control.RawQuery = query.Encode()
	return control.String(), nil
}

func normalizeTabletCommandPayload(commandType string, raw json.RawMessage) (json.RawMessage, error) {
	commandType = strings.TrimSpace(commandType)
	if commandType == deviceexperience.CommandTabletBrightnessSet {
		var object map[string]any
		if len(raw) == 0 || json.Unmarshal(raw, &object) != nil || object == nil || len(object) != 1 {
			return nil, fmt.Errorf("brightness requires exactly brightness_percent")
		}
		value, ok := object["brightness_percent"].(float64)
		if !ok || value != float64(int(value)) || value < 5 || value > 100 {
			return nil, fmt.Errorf("brightness_percent must be an integer between 5 and 100")
		}
		encoded, _ := json.Marshal(map[string]int{"brightness_percent": int(value)})
		return encoded, nil
	}
	if commandType == deviceexperience.CommandTabletShowModeSet {
		var object map[string]any
		if len(raw) == 0 || json.Unmarshal(raw, &object) != nil || object == nil || len(object) != 1 {
			return nil, fmt.Errorf("Show Mode requires exactly show_mode")
		}
		value, ok := object["show_mode"].(bool)
		if !ok {
			return nil, fmt.Errorf("show_mode must be boolean")
		}
		encoded, _ := json.Marshal(map[string]bool{"show_mode": value})
		return encoded, nil
	}
	if commandType == deviceexperience.CommandTabletVideoScaleSet {
		var object map[string]any
		if len(raw) == 0 || json.Unmarshal(raw, &object) != nil || object == nil || len(object) != 1 {
			return nil, fmt.Errorf("video scale requires exactly video_scale_mode")
		}
		value, ok := object["video_scale_mode"].(string)
		value = strings.ToUpper(strings.TrimSpace(value))
		if !ok || (value != "FIT" && value != "CROP" && value != "FULL") {
			return nil, fmt.Errorf("video_scale_mode must be FIT, CROP, or FULL")
		}
		encoded, _ := json.Marshal(map[string]string{"video_scale_mode": value})
		return encoded, nil
	}
	if commandType == deviceexperience.CommandTabletOrientationSet {
		var object map[string]any
		if len(raw) == 0 || json.Unmarshal(raw, &object) != nil || object == nil || len(object) != 1 {
			return nil, fmt.Errorf("orientation requires exactly orientation_mode")
		}
		value, ok := object["orientation_mode"].(string)
		value = strings.ToUpper(strings.TrimSpace(value))
		if !ok || (value != "AUTO" && value != "PORTRAIT" && value != "LANDSCAPE") {
			return nil, fmt.Errorf("orientation_mode must be AUTO, PORTRAIT, or LANDSCAPE")
		}
		encoded, _ := json.Marshal(map[string]string{"orientation_mode": value})
		return encoded, nil
	}
	if commandType == deviceexperience.CommandTabletLiveRotationSet {
		var object map[string]any
		if len(raw) == 0 || json.Unmarshal(raw, &object) != nil || object == nil || len(object) != 1 {
			return nil, fmt.Errorf("Live rotation requires exactly live_rotation_degrees")
		}
		value, ok := object["live_rotation_degrees"].(float64)
		if !ok || value != float64(int(value)) {
			return nil, fmt.Errorf("live_rotation_degrees must be 0, 90, 180, or 270")
		}
		degrees := int(value)
		if degrees != 0 && degrees != 90 && degrees != 180 && degrees != 270 {
			return nil, fmt.Errorf("live_rotation_degrees must be 0, 90, 180, or 270")
		}
		encoded, _ := json.Marshal(map[string]int{"live_rotation_degrees": degrees})
		return encoded, nil
	}
	if commandType == deviceexperience.CommandTabletPlay {
		var object map[string]any
		if len(raw) == 0 || json.Unmarshal(raw, &object) != nil || object == nil {
			return nil, fmt.Errorf("playback payload must be an object")
		}
		allowed := map[string]bool{
			"tablet_cue_id": true,
			"tablet_sequence": true,
			"media_number": true,
			"loop": true,
			"end_behavior": true,
		}
		for key := range object {
			if !allowed[key] {
				return nil, fmt.Errorf("unsupported playback field %q", key)
			}
		}
		selectors := 0
		if _, ok := object["tablet_cue_id"]; ok { selectors++ }
		if _, ok := object["tablet_sequence"]; ok { selectors++ }
		if _, ok := object["media_number"]; ok { selectors++ }
		if selectors != 1 {
			return nil, fmt.Errorf("TABLET_PLAY requires exactly one media selector")
		}
		if cueRaw, ok := object["tablet_cue_id"]; ok {
			cueID, ok := cueRaw.(string)
			cueID = strings.TrimSpace(cueID)
			if !ok || cueID == "" || len(cueID) > 256 {
				return nil, fmt.Errorf("tablet_cue_id must be a non-empty string")
			}
			if _, exists := object["loop"]; exists {
				return nil, fmt.Errorf("loop is only valid with media_number")
			}
			if _, exists := object["end_behavior"]; exists {
				return nil, fmt.Errorf("end_behavior is only valid with media_number")
			}
			encoded, _ := json.Marshal(map[string]string{"tablet_cue_id": cueID})
			return encoded, nil
		}
		if sequenceRaw, ok := object["tablet_sequence"]; ok {
			sequence, ok := sequenceRaw.(float64)
			if !ok || sequence != float64(int(sequence)) || sequence < 1 {
				return nil, fmt.Errorf("tablet_sequence must be a positive integer")
			}
			if _, exists := object["loop"]; exists {
				return nil, fmt.Errorf("loop is only valid with media_number")
			}
			if _, exists := object["end_behavior"]; exists {
				return nil, fmt.Errorf("end_behavior is only valid with media_number")
			}
			encoded, _ := json.Marshal(map[string]int{"tablet_sequence": int(sequence)})
			return encoded, nil
		}
		mediaRaw, ok := object["media_number"]
		mediaNumber, ok := mediaRaw.(float64)
		if !ok || mediaNumber != float64(int(mediaNumber)) || mediaNumber < 1 {
			return nil, fmt.Errorf("media_number must be a positive integer")
		}
		normalized := map[string]any{"media_number": int(mediaNumber)}
		if loopRaw, exists := object["loop"]; exists {
			loop, ok := loopRaw.(bool)
			if !ok {
				return nil, fmt.Errorf("loop must be boolean")
			}
			normalized["loop"] = loop
		}
		if endRaw, exists := object["end_behavior"]; exists {
			endBehavior, ok := endRaw.(string)
			endBehavior = strings.ToLower(strings.TrimSpace(endBehavior))
			if !ok || (endBehavior != "none" && endBehavior != "hold" && endBehavior != "blackout" && endBehavior != "stop" && endBehavior != "clear") {
				return nil, fmt.Errorf("end_behavior must be none, hold, blackout, stop, or clear")
			}
			normalized["end_behavior"] = endBehavior
		}
		encoded, _ := json.Marshal(normalized)
		return encoded, nil
	}
	if commandType != deviceexperience.CommandTabletLiveShow {
		return normalizeRawObject(raw), nil
	}
	var object map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &object) != nil || object == nil {
		return nil, fmt.Errorf("live source requires exactly one of media_key or url")
	}
	for key := range object {
		if key != "media_key" && key != "url" {
			return nil, fmt.Errorf("unsupported live source field %q", key)
		}
	}
	mediaKey, _ := object["media_key"].(string)
	directURL, _ := object["url"].(string)
	mediaKey = strings.TrimSpace(mediaKey)
	directURL = strings.TrimSpace(directURL)
	if (mediaKey == "") == (directURL == "") {
		return nil, fmt.Errorf("live source requires exactly one of media_key or url")
	}
	if mediaKey != "" {
		if len(mediaKey) > 256 {
			return nil, fmt.Errorf("live media_key is too long")
		}
		encoded, _ := json.Marshal(map[string]string{"media_key": mediaKey})
		return encoded, nil
	}
	if len(directURL) > 2048 {
		return nil, fmt.Errorf("live URL is too long")
	}
	parsed, err := url.Parse(directURL)
	if err != nil || parsed.Host == "" || parsed.User != nil ||
		(!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) {
		return nil, fmt.Errorf("live URL must be absolute HTTP(S) without embedded credentials")
	}
	encoded, _ := json.Marshal(map[string]string{"url": directURL})
	return encoded, nil
}

func tabletRuntimeCapability(capability string) bool {
	return strings.HasPrefix(capability, "tablet.media.") ||
		capability == deviceexperience.CapabilityTabletBrightnessSet ||
		capability == deviceexperience.CapabilityTabletShowModeSet ||
		capability == deviceexperience.CapabilityTabletVideoScaleSet ||
		capability == deviceexperience.CapabilityTabletOrientationSet ||
		capability == deviceexperience.CapabilityTabletLiveRotationSet
}

func tabletSettingsCommand(commandType string) bool {
	switch strings.TrimSpace(commandType) {
	case deviceexperience.CommandTabletBrightnessSet,
		deviceexperience.CommandTabletShowModeSet,
		deviceexperience.CommandTabletVideoScaleSet,
		deviceexperience.CommandTabletOrientationSet,
		deviceexperience.CommandTabletLiveRotationSet:
		return true
	default:
		return false
	}
}

func normalizeRawObject(raw json.RawMessage) json.RawMessage {
	var object map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &object) != nil || object == nil {
		return json.RawMessage(`{}`)
	}
	encoded, err := json.Marshal(object)
	if err != nil { return json.RawMessage(`{}`) }
	return encoded
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] { continue }
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted { return true }
	}
	return false
}
