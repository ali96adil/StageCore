package deviceexperience

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrStageLaserVisualStateChanged = errors.New("stagelaser device state changed since the operator opened the visual check")

// StageLaserVisualCheck is an independently logged *human observation*.
// It never rewrites observed_state_json, state_quality, hardware feedback,
// assignment, snapshot, readiness or command authority.
type StageLaserVisualCheck struct {
	CheckID               string    `json:"check_id"`
	DeviceID              string    `json:"device_id"`
	ProjectID             string    `json:"project_id"`
	ActorID               string    `json:"actor_id"`
	VisualState           string    `json:"visual_state"`
	DeviceReportedState   string    `json:"device_reported_state"`
	DeviceConnectionState string    `json:"device_connection_state"`
	Comparison            string    `json:"comparison"`
	CheckedAt             time.Time `json:"checked_at"`
}

func stageLaserComparison(visual, reported string) string {
	if visual != "ON" && visual != "OFF" {
		return "UNVERIFIED"
	}
	if reported != "ON" && reported != "OFF" {
		return "UNVERIFIED"
	}
	if visual == reported {
		return "MATCH"
	}
	return "MISMATCH"
}

// RecordStageLaserVisualCheck does not issue any Stage Device command.
// The reported logical state comes from the DB inside the same transaction,
// never from a browser-provided or manually overridden value.
func (r *Repository) RecordStageLaserVisualCheck(
	ctx context.Context, deviceID, projectID, actorID, visual, expectedReported string,
) (StageLaserVisualCheck, error) {
	deviceID, projectID, actorID = strings.TrimSpace(deviceID), strings.TrimSpace(projectID), strings.TrimSpace(actorID)
	visual = strings.ToUpper(strings.TrimSpace(visual))
	expectedReported = strings.ToUpper(strings.TrimSpace(expectedReported))
	if deviceID == "" || projectID == "" || actorID == "" ||
		(visual != "ON" && visual != "OFF" && visual != "UNKNOWN") ||
		expectedReported == "" {
		return StageLaserVisualCheck{}, ErrInvalidDevice
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return StageLaserVisualCheck{}, err
	}
	defer tx.Rollback()

	var protocol, kind, profile, connection string
	var stateJSON sql.NullString
	if err = tx.QueryRowContext(ctx, `
		SELECT d.protocol_version, d.device_kind, COALESCE(d.profile_id, ''),
		       COALESCE(s.connection_state, 'NO_RUNTIME'), s.observed_state_json
		FROM stage_devices d
		LEFT JOIN stage_device_runtime_state s ON s.device_id = d.device_id
		WHERE d.device_id = ? AND d.enabled = 1
	`, deviceID).Scan(&protocol, &kind, &profile, &connection, &stateJSON); err != nil {
		return StageLaserVisualCheck{}, err
	}
	if protocol != ProtocolVersion2 || kind != string(DeviceGeneric) ||
		profile != "stagecore.esp32-stagelaser" {
		return StageLaserVisualCheck{}, ErrInvalidDevice
	}
	reported := "UNKNOWN"
	if stateJSON.Valid {
		var observed struct {
			LogicalState string `json:"logical_state"`
		}
		if json.Unmarshal([]byte(stateJSON.String), &observed) == nil {
			if state := strings.ToUpper(strings.TrimSpace(observed.LogicalState)); state != "" {
				reported = state
			}
		}
	}
	if expectedReported != reported {
		return StageLaserVisualCheck{}, ErrStageLaserVisualStateChanged
	}
	checkedAt := r.now().UTC()
	check := StageLaserVisualCheck{
		CheckID: uuid.NewString(), DeviceID: deviceID, ProjectID: projectID,
		ActorID: actorID, VisualState: visual, DeviceReportedState: reported,
		DeviceConnectionState: connection, Comparison: stageLaserComparison(visual, reported),
		CheckedAt: checkedAt,
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO stage_device_stagelaser_visual_checks
			(check_id, device_id, project_id, actor_id, visual_state,
			 device_reported_state, device_connection_state, checked_at_us)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, check.CheckID, check.DeviceID, check.ProjectID, check.ActorID,
		check.VisualState, check.DeviceReportedState, check.DeviceConnectionState,
		checkedAt.UnixMicro()); err != nil {
		return StageLaserVisualCheck{}, fmt.Errorf("record StageLaser visual check: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return StageLaserVisualCheck{}, err
	}
	return check, nil
}

func (r *Repository) LatestStageLaserVisualCheck(
	ctx context.Context, deviceID, projectID string,
) (*StageLaserVisualCheck, error) {
	if strings.TrimSpace(deviceID) == "" || strings.TrimSpace(projectID) == "" {
		return nil, ErrInvalidDevice
	}
	var check StageLaserVisualCheck
	var checkedAtUS int64
	err := r.db.QueryRowContext(ctx, `
		SELECT check_id, device_id, project_id, actor_id, visual_state,
		       device_reported_state, device_connection_state, checked_at_us
		FROM stage_device_stagelaser_visual_checks
		WHERE device_id = ? AND project_id = ?
		ORDER BY checked_at_us DESC, check_id DESC LIMIT 1
	`, deviceID, projectID).Scan(
		&check.CheckID, &check.DeviceID, &check.ProjectID, &check.ActorID,
		&check.VisualState, &check.DeviceReportedState, &check.DeviceConnectionState,
		&checkedAtUS,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	check.Comparison = stageLaserComparison(check.VisualState, check.DeviceReportedState)
	check.CheckedAt = time.UnixMicro(checkedAtUS).UTC()
	return &check, nil
}
