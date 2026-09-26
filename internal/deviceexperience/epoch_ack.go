package deviceexperience

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/lightingnode"
)

// BlockedEpochAck records a device-reported software-zero state for the
// CURRENT Hub-committed BLOCKED epoch. It never grants ACTIVE/readiness,
// snapshot or lighting commands, and cannot prove a physical fixture is dark.
type BlockedEpochAck struct {
	DeviceID             string    `json:"device_id"`
	AssignmentEpoch      int64     `json:"assignment_epoch"`
	ProjectID            string    `json:"project_id"`
	ConnectionGeneration int64     `json:"connection_generation"`
	ChannelCount         int       `json:"channel_count"`
	AcknowledgedAt       time.Time `json:"acknowledged_at"`
}

// RecordBlockedEpochAck MUST be called only after validating an authenticated
// CURRENT v2 WebSocket; it is intentionally not exposed through HTTP.
// DB triggers recheck committed audit, current sidecar and pending commands.
func (r *Repository) RecordBlockedEpochAck(
	ctx context.Context, deviceID, projectID string,
	epoch, generation int64, blackout bool, channels []int,
) (BlockedEpochAck, error) {
	deviceID = strings.TrimSpace(deviceID)
	projectID = strings.TrimSpace(projectID)
	if deviceID == "" || projectID == "" || epoch <= 1 ||
		generation < 1 || !blackout || len(channels) != lightingnode.MaxChannels {
		return BlockedEpochAck{}, fmt.Errorf("%w: invalid blocked epoch software acknowledgment", ErrInvalidState)
	}
	for _, level := range channels {
		if level != 0 {
			return BlockedEpochAck{}, fmt.Errorf("%w: epoch ACK contains nonzero channel", ErrInvalidState)
		}
	}
	nowUS := r.now().UTC().UnixMicro()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO stage_device_epoch_acks
		(device_id, assignment_epoch, project_id, connection_generation,
		 channel_count, acked_at_us)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(device_id, assignment_epoch) DO UPDATE SET
		    connection_generation=excluded.connection_generation,
		    acked_at_us=excluded.acked_at_us
		WHERE stage_device_epoch_acks.project_id=excluded.project_id
	`, deviceID, epoch, projectID, generation, len(channels), nowUS)
	if err != nil {
		return BlockedEpochAck{}, fmt.Errorf("%w: epoch ACK conflicts with current Hub assignment: %v", ErrInvalidState, err)
	}
	return r.GetBlockedEpochAck(ctx, deviceID, epoch)
}

// GetBlockedEpochAck is read-only. A persisted ACK on an earlier socket is
// historical evidence; future activation MUST validate a fresh generation.
func (r *Repository) GetBlockedEpochAck(ctx context.Context, deviceID string, epoch int64) (BlockedEpochAck, error) {
	var out BlockedEpochAck
	var us int64
	err := r.db.QueryRowContext(ctx, `
		SELECT device_id, assignment_epoch, project_id,
		       connection_generation, channel_count, acked_at_us
		FROM stage_device_epoch_acks WHERE device_id=? AND assignment_epoch=?
	`, strings.TrimSpace(deviceID), epoch).Scan(
		&out.DeviceID, &out.AssignmentEpoch, &out.ProjectID,
		&out.ConnectionGeneration, &out.ChannelCount, &us)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BlockedEpochAck{}, err
		}
		return BlockedEpochAck{}, fmt.Errorf("read v2 epoch ACK: %w", err)
	}
	out.AcknowledgedAt = time.UnixMicro(us).UTC()
	return out, nil
}
