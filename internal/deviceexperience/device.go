package deviceexperience

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (r *Repository) UpsertDevice(ctx context.Context, device Device) (Device, error) {
	device.ID = strings.TrimSpace(device.ID)
	device.ProjectID = strings.TrimSpace(device.ProjectID)
	device.ProfileID = strings.TrimSpace(device.ProfileID)
	device.DisplayName = strings.TrimSpace(device.DisplayName)
	device.Platform = strings.TrimSpace(device.Platform)
	device.Architecture = strings.TrimSpace(device.Architecture)
	device.ClientVersion = strings.TrimSpace(device.ClientVersion)
	device.ProtocolVersion = strings.TrimSpace(device.ProtocolVersion)
	device.GroupName = strings.TrimSpace(device.GroupName)
	device.LocationName = strings.TrimSpace(device.LocationName)
	device.Capabilities = normalizeCapabilities(device.Capabilities)
	if device.ID == "" || device.DisplayName == "" || !validDeviceKind(device.Kind) {
		return Device{}, ErrInvalidDevice
	}
	if device.ProtocolVersion == "" {
		device.ProtocolVersion = ProtocolVersion1
	}
	if device.ProtocolVersion != ProtocolVersion1 {
		return Device{}, fmt.Errorf("%w: unsupported protocol %q", ErrInvalidDevice, device.ProtocolVersion)
	}
	caps, err := json.Marshal(device.Capabilities)
	if err != nil {
		return Device{}, fmt.Errorf("encode device capabilities: %w", err)
	}
	now := r.now().UTC()
	nowUS := now.UnixMicro()
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO stage_devices
		(device_id, project_id, profile_id, device_kind, display_name, platform, architecture,
		 client_version, protocol_version, capabilities_json, group_name, location_name, enabled,
		 created_at_us, updated_at_us)
		VALUES (?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(device_id) DO UPDATE SET
		project_id=excluded.project_id,
		profile_id=excluded.profile_id,
		device_kind=excluded.device_kind,
		display_name=excluded.display_name,
		platform=excluded.platform,
		architecture=excluded.architecture,
		client_version=excluded.client_version,
		protocol_version=excluded.protocol_version,
		capabilities_json=excluded.capabilities_json,
		group_name=excluded.group_name,
		location_name=excluded.location_name,
		enabled=excluded.enabled,
		updated_at_us=excluded.updated_at_us
	`, device.ID, device.ProjectID, device.ProfileID, device.Kind, device.DisplayName,
		device.Platform, device.Architecture, device.ClientVersion, device.ProtocolVersion,
		string(caps), device.GroupName, device.LocationName, boolInt(device.Enabled), nowUS, nowUS)
	if err != nil {
		return Device{}, fmt.Errorf("upsert stage device: %w", err)
	}
	return r.GetDevice(ctx, device.ID)
}

func (r *Repository) GetDevice(ctx context.Context, deviceID string) (Device, error) {
	var device Device
	var projectID, profileID sql.NullString
	var capsJSON string
	var enabled int
	var createdUS, updatedUS int64
	err := r.db.QueryRowContext(ctx, `
		SELECT device_id, project_id, profile_id, device_kind, display_name, platform, architecture,
		       client_version, protocol_version, capabilities_json, group_name, location_name,
		       enabled, created_at_us, updated_at_us
		FROM stage_devices WHERE device_id = ?
	`, strings.TrimSpace(deviceID)).Scan(
		&device.ID, &projectID, &profileID, &device.Kind, &device.DisplayName, &device.Platform,
		&device.Architecture, &device.ClientVersion, &device.ProtocolVersion, &capsJSON,
		&device.GroupName, &device.LocationName, &enabled, &createdUS, &updatedUS,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, sql.ErrNoRows
	}
	if err != nil {
		return Device{}, fmt.Errorf("get stage device: %w", err)
	}
	if projectID.Valid {
		device.ProjectID = projectID.String
	}
	if profileID.Valid {
		device.ProfileID = profileID.String
	}
	device.Enabled = enabled == 1
	device.CreatedAt = time.UnixMicro(createdUS).UTC()
	device.UpdatedAt = time.UnixMicro(updatedUS).UTC()
	if err := json.Unmarshal([]byte(capsJSON), &device.Capabilities); err != nil {
		return Device{}, fmt.Errorf("decode device capabilities: %w", err)
	}
	state, err := r.getRuntimeState(ctx, device.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Device{}, err
	}
	if err == nil {
		device.Runtime = &state
	}
	return device, nil
}

func (r *Repository) ListDevices(ctx context.Context, projectID string) ([]Device, error) {
	projectID = strings.TrimSpace(projectID)
	query := `SELECT device_id FROM stage_devices`
	args := []any{}
	if projectID != "" {
		query += ` WHERE project_id = ?`
		args = append(args, projectID)
	}
	query += ` ORDER BY display_name COLLATE NOCASE, device_id`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list stage devices: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan stage device id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Device, 0, len(ids))
	for _, id := range ids {
		device, err := r.GetDevice(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, device)
	}
	return out, nil
}

func (r *Repository) ObserveDevice(ctx context.Context, observation RuntimeObservation) (RuntimeState, error) {
	observation.DeviceID = strings.TrimSpace(observation.DeviceID)
	if observation.DeviceID == "" || !validConnectionState(observation.Connection) || !validReadiness(observation.Readiness) {
		return RuntimeState{}, ErrInvalidState
	}
	if _, err := r.GetDevice(ctx, observation.DeviceID); err != nil {
		return RuntimeState{}, err
	}
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = r.now().UTC()
	} else {
		observation.ObservedAt = observation.ObservedAt.UTC()
	}
	observed := normalizeJSON(observation.ObservedState, `{}`)
	network := normalizeJSON(observation.NetworkState, `{}`)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO stage_device_runtime_state
		(device_id, connection_state, readiness, last_seen_at_us, observed_state_json, network_state_json)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(device_id) DO UPDATE SET
		connection_state=excluded.connection_state,
		readiness=excluded.readiness,
		last_seen_at_us=excluded.last_seen_at_us,
		observed_state_json=excluded.observed_state_json,
		network_state_json=excluded.network_state_json
	`, observation.DeviceID, observation.Connection, observation.Readiness, observation.ObservedAt.UnixMicro(), string(observed), string(network))
	if err != nil {
		return RuntimeState{}, fmt.Errorf("observe stage device: %w", err)
	}
	return r.getRuntimeState(ctx, observation.DeviceID)
}

func (r *Repository) getRuntimeState(ctx context.Context, deviceID string) (RuntimeState, error) {
	var state RuntimeState
	var lastSeenUS int64
	var observed, network string
	err := r.db.QueryRowContext(ctx, `
		SELECT device_id, connection_state, readiness, last_seen_at_us, observed_state_json, network_state_json
		FROM stage_device_runtime_state WHERE device_id = ?
	`, deviceID).Scan(&state.DeviceID, &state.Connection, &state.Readiness, &lastSeenUS, &observed, &network)
	if err != nil {
		return RuntimeState{}, err
	}
	state.LastSeenAt = time.UnixMicro(lastSeenUS).UTC()
	state.ObservedState = json.RawMessage(observed)
	state.NetworkState = json.RawMessage(network)
	return state, nil
}
