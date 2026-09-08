package deviceexperience

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	stageid "github.com/ali96adil/StageCore/internal/id"
)

func (r *Repository) UpsertLiveSource(ctx context.Context, source LiveSource) (LiveSource, error) {
	source.ID = strings.TrimSpace(source.ID)
	source.ProjectID = strings.TrimSpace(source.ProjectID)
	source.Name = strings.TrimSpace(source.Name)
	source.ExecutionDeviceID = strings.TrimSpace(source.ExecutionDeviceID)
	source.ProfileID = strings.TrimSpace(source.ProfileID)
	source.EndpointRef = strings.TrimSpace(source.EndpointRef)
	source.Capabilities = normalizeCapabilities(source.Capabilities)
	if source.ProjectID == "" || source.Name == "" || !validSourceClass(source.Class) {
		return LiveSource{}, ErrInvalidState
	}
	if source.ID == "" {
		id, err := stageid.New()
		if err != nil {
			return LiveSource{}, err
		}
		source.ID = id
	}
	if source.Readiness == "" {
		source.Readiness = ReadinessUnknown
	}
	if !validReadiness(source.Readiness) {
		return LiveSource{}, ErrInvalidState
	}
	if source.ExecutionDeviceID != "" {
		if _, err := r.GetDevice(ctx, source.ExecutionDeviceID); err != nil {
			return LiveSource{}, err
		}
	}
	caps, err := json.Marshal(source.Capabilities)
	if err != nil {
		return LiveSource{}, err
	}
	source.Config = normalizeJSON(source.Config, `{}`)
	now := r.now().UTC()
	var executionDevice, profile, lastObserved any
	if source.ExecutionDeviceID != "" {
		executionDevice = source.ExecutionDeviceID
	}
	if source.ProfileID != "" {
		profile = source.ProfileID
	}
	if source.LastObservedAt != nil {
		last := source.LastObservedAt.UTC()
		source.LastObservedAt = &last
		lastObserved = last.UnixMicro()
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO live_video_sources
		(source_id, project_id, name, source_class, execution_device_id, profile_id, endpoint_ref,
		 capabilities_json, config_json, required, desired_enabled, readiness, last_observed_at_us,
		 created_at_us, updated_at_us)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_id) DO UPDATE SET
		project_id=excluded.project_id, name=excluded.name, source_class=excluded.source_class,
		execution_device_id=excluded.execution_device_id, profile_id=excluded.profile_id,
		endpoint_ref=excluded.endpoint_ref, capabilities_json=excluded.capabilities_json,
		config_json=excluded.config_json, required=excluded.required,
		desired_enabled=excluded.desired_enabled, readiness=excluded.readiness,
		last_observed_at_us=excluded.last_observed_at_us, updated_at_us=excluded.updated_at_us
	`, source.ID, source.ProjectID, source.Name, source.Class, executionDevice, profile, source.EndpointRef,
		string(caps), string(source.Config), boolInt(source.Required), boolInt(source.DesiredEnabled), source.Readiness,
		lastObserved, now.UnixMicro(), now.UnixMicro())
	if err != nil {
		return LiveSource{}, fmt.Errorf("upsert live video source: %w", err)
	}
	return r.GetLiveSource(ctx, source.ID)
}

func (r *Repository) GetLiveSource(ctx context.Context, sourceID string) (LiveSource, error) {
	var source LiveSource
	var executionDevice, profile sql.NullString
	var caps, config string
	var required, desired int
	var lastObserved sql.NullInt64
	var createdUS, updatedUS int64
	err := r.db.QueryRowContext(ctx, `
		SELECT source_id, project_id, name, source_class, execution_device_id, profile_id, endpoint_ref,
		       capabilities_json, config_json, required, desired_enabled, readiness, last_observed_at_us,
		       created_at_us, updated_at_us
		FROM live_video_sources WHERE source_id = ?
	`, strings.TrimSpace(sourceID)).Scan(&source.ID, &source.ProjectID, &source.Name, &source.Class,
		&executionDevice, &profile, &source.EndpointRef, &caps, &config, &required, &desired,
		&source.Readiness, &lastObserved, &createdUS, &updatedUS)
	if err != nil {
		return LiveSource{}, err
	}
	if executionDevice.Valid {
		source.ExecutionDeviceID = executionDevice.String
	}
	if profile.Valid {
		source.ProfileID = profile.String
	}
	if err := json.Unmarshal([]byte(caps), &source.Capabilities); err != nil {
		return LiveSource{}, err
	}
	source.Config = json.RawMessage(config)
	source.Required = required == 1
	source.DesiredEnabled = desired == 1
	if lastObserved.Valid {
		value := time.UnixMicro(lastObserved.Int64).UTC()
		source.LastObservedAt = &value
	}
	source.CreatedAt = time.UnixMicro(createdUS).UTC()
	source.UpdatedAt = time.UnixMicro(updatedUS).UTC()
	return source, nil
}

func (r *Repository) ListLiveSources(ctx context.Context, projectID string) ([]LiveSource, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT source_id FROM live_video_sources WHERE project_id = ? ORDER BY name COLLATE NOCASE, source_id`, strings.TrimSpace(projectID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	out := make([]LiveSource, 0, len(ids))
	for _, id := range ids {
		source, err := r.GetLiveSource(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, source)
	}
	return out, rows.Err()
}
