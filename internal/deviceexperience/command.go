package deviceexperience

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

func (r *Repository) CreateCommand(ctx context.Context, input CreateCommandInput) (DeviceCommand, bool, error) {
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.CommandType = strings.TrimSpace(input.CommandType)
	input.Issuer = strings.TrimSpace(input.Issuer)
	input.CorrelationID = strings.TrimSpace(input.CorrelationID)
	input.CausationID = strings.TrimSpace(input.CausationID)
	input.RuntimeSnapshotID = strings.TrimSpace(input.RuntimeSnapshotID)
	input.Priority = strings.TrimSpace(input.Priority)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.ProjectID == "" || input.DeviceID == "" || input.CommandType == "" || input.Issuer == "" {
		return DeviceCommand{}, false, fmt.Errorf("%w: project, device, command type and issuer are required", ErrInvalidState)
	}
	capability := RequiredCapability(input.CommandType)
	if capability == "" {
		return DeviceCommand{}, false, fmt.Errorf("%w: unsupported command type %q", ErrInvalidState, input.CommandType)
	}
	device, err := r.GetDevice(ctx, input.DeviceID)
	if err != nil {
		return DeviceCommand{}, false, err
	}
	// Command authority is always Hub-owned. Legacy v1 devices still use the
	// legacy project row plus LEGACY sidecar. Project-independent v2 Tablet
	// Players use only an ACTIVE sidecar with the exact Project + Snapshot.
	if !device.Enabled {
		return DeviceCommand{}, false, fmt.Errorf("%w: device is disabled", ErrInvalidDevice)
	}
	assignment, err := r.GetAssignmentRecord(ctx, input.DeviceID)
	if err != nil {
		return DeviceCommand{}, false, fmt.Errorf("%w: assignment metadata unavailable: %v", ErrInvalidState, err)
	}
	switch device.ProtocolVersion {
	case ProtocolVersion1:
		if device.ProjectID == "" || device.ProjectID != input.ProjectID ||
			assignment.State != AssignmentLegacy || assignment.ProjectID != input.ProjectID {
			return DeviceCommand{}, false, fmt.Errorf("%w: legacy command authority does not match Project", ErrInvalidState)
		}
	case ProtocolVersion2:
		if device.ProjectID != "" || device.Kind != DeviceTabletPlayer ||
			device.ProfileID != TabletPlayerProfileID ||
			assignment.State != "ACTIVE" ||
			assignment.ProjectID != input.ProjectID ||
			assignment.RuntimeSnapshotID == "" ||
			assignment.RuntimeSnapshotID != input.RuntimeSnapshotID {
			return DeviceCommand{}, false, fmt.Errorf("%w: v2 Tablet Player is not ACTIVE for this Project/Runtime Snapshot", ErrInvalidState)
		}
	default:
		return DeviceCommand{}, false, fmt.Errorf("%w: unsupported Stage Device protocol", ErrInvalidState)
	}
	if !contains(device.Capabilities, capability) {
		return DeviceCommand{}, false, fmt.Errorf("%w: %s", ErrCapabilityMissing, capability)
	}
	now := r.now().UTC()
	if input.DeadlineAt != nil {
		deadline := input.DeadlineAt.UTC()
		input.DeadlineAt = &deadline
		if !deadline.After(now) {
			return DeviceCommand{}, false, ErrCommandExpired
		}
	}
	if input.IdempotencyKey != "" {
		existing, err := r.getCommandByIdempotency(ctx, input.DeviceID, input.IdempotencyKey)
		if err == nil {
			return existing, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return DeviceCommand{}, false, err
		}
	}
	payload, err := canonicalCommandPayload(input.CommandType, input.Payload, now)
	if err != nil {
		return DeviceCommand{}, false, err
	}
	if err := r.validateLightingCommandAuthority(ctx, input, device, payload); err != nil {
		return DeviceCommand{}, false, err
	}
	commandID, err := stageid.New()
	if err != nil {
		return DeviceCommand{}, false, err
	}
	if input.Priority == "" {
		input.Priority = "P1"
	}
	envelope := contracts.CommandEnvelope{
		CommandID:         commandID,
		CommandType:       input.CommandType,
		SchemaVersion:     contracts.SchemaVersion1,
		IssuedAt:          now,
		DeadlineAt:        input.DeadlineAt,
		ProjectID:         input.ProjectID,
		RuntimeSnapshotID: input.RuntimeSnapshotID,
		Issuer:            input.Issuer,
		CorrelationID:     input.CorrelationID,
		CausationID:       input.CausationID,
		Priority:          input.Priority,
		IdempotencyKey:    input.IdempotencyKey,
		Payload:           payload,
	}
	var deadline any
	if envelope.DeadlineAt != nil {
		deadline = envelope.DeadlineAt.UnixMicro()
	}
	var session any
	if input.SessionID != "" {
		session = input.SessionID
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO stage_device_commands
		(command_id, project_id, session_id, device_id, command_type, runtime_snapshot_id, issued_at_us, deadline_at_us,
		 issuer, correlation_id, causation_id, priority, idempotency_key, payload_json, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'ACCEPTED')
	`, commandID, input.ProjectID, session, input.DeviceID, input.CommandType,
		input.RuntimeSnapshotID, now.UnixMicro(), deadline,
		input.Issuer, input.CorrelationID, input.CausationID, input.Priority, input.IdempotencyKey, string(payload))
	if err != nil {
		if input.IdempotencyKey != "" {
			existing, getErr := r.getCommandByIdempotency(ctx, input.DeviceID, input.IdempotencyKey)
			if getErr == nil {
				return existing, true, nil
			}
		}
		return DeviceCommand{}, false, fmt.Errorf("create stage device command: %w", err)
	}
	created := DeviceCommand{Envelope: envelope, SessionID: input.SessionID, DeviceID: input.DeviceID, Status: contracts.CommandAccepted}
	if err := r.recordCommandEvent(ctx, created, "stage_device.command.accepted"); err != nil {
		_, _ = r.db.ExecContext(context.WithoutCancel(ctx), `DELETE FROM stage_device_commands WHERE command_id = ? AND status = 'ACCEPTED'`, commandID)
		return DeviceCommand{}, false, fmt.Errorf("record stage device command accepted event: %w", err)
	}
	return created, false, nil
}

func (r *Repository) CompleteCommand(ctx context.Context, commandID string, status contracts.CommandStatus, result json.RawMessage) (DeviceCommand, error) {
	commandID = strings.TrimSpace(commandID)
	if commandID == "" || !terminalCommandStatus(status) {
		return DeviceCommand{}, ErrInvalidState
	}

	// A terminal command must not become observable to the cue forwarder before
	// its canonical flight-recorder event is durable. Otherwise the cue engine
	// can emit action.completed first and produce a causally inverted event log.
	r.commandResultMu.Lock()
	defer r.commandResultMu.Unlock()

	now := r.now().UTC()
	result = normalizeJSON(result, `{}`)
	res, err := r.db.ExecContext(ctx, `
		UPDATE stage_device_commands
		SET status = ?, result_json = ?, completed_at_us = ?
		WHERE command_id = ? AND status = 'ACCEPTED'
	`, status, string(result), now.UnixMicro(), commandID)
	if err != nil {
		return DeviceCommand{}, fmt.Errorf("complete stage device command: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return DeviceCommand{}, err
	}
	if rows == 0 {
		existing, getErr := r.getCommandUnlocked(ctx, commandID)
		if getErr != nil {
			return DeviceCommand{}, getErr
		}
		if terminalCommandStatus(existing.Status) {
			return existing, nil
		}
		return DeviceCommand{}, ErrInvalidState
	}
	completed, err := r.getCommandUnlocked(ctx, commandID)
	if err != nil {
		return DeviceCommand{}, err
	}
	if err := r.recordCommandEvent(ctx, completed, commandResultEventType(status)); err != nil {
		rollbackCtx := context.WithoutCancel(ctx)
		if _, rollbackErr := r.db.ExecContext(rollbackCtx, `
			UPDATE stage_device_commands
			SET status = 'ACCEPTED', result_json = NULL, completed_at_us = NULL
			WHERE command_id = ? AND status = ? AND completed_at_us = ?
		`, commandID, status, now.UnixMicro()); rollbackErr != nil {
			return completed, fmt.Errorf("record stage device command result event: %w; rollback terminal command: %v", err, rollbackErr)
		}
		return DeviceCommand{}, fmt.Errorf("record stage device command result event: %w", err)
	}
	if status == contracts.CommandCompleted {
		if err := r.applyCompletedDisplayCommand(ctx, completed); err != nil {
			return completed, fmt.Errorf("update safe Stage Display reconnect state: %w", err)
		}
	}
	return completed, nil
}

func (r *Repository) GetCommand(ctx context.Context, commandID string) (DeviceCommand, error) {
	r.commandResultMu.RLock()
	defer r.commandResultMu.RUnlock()
	return r.getCommandUnlocked(ctx, commandID)
}

func (r *Repository) getCommandUnlocked(ctx context.Context, commandID string) (DeviceCommand, error) {
	return r.scanCommand(r.db.QueryRowContext(ctx, `
		SELECT command_id, project_id, session_id, device_id, command_type, runtime_snapshot_id, issued_at_us, deadline_at_us,
		       issuer, correlation_id, causation_id, priority, idempotency_key, payload_json, status,
		       result_json, completed_at_us
		FROM stage_device_commands WHERE command_id = ?
	`, strings.TrimSpace(commandID)))
}

func (r *Repository) getCommandByIdempotency(ctx context.Context, deviceID, key string) (DeviceCommand, error) {
	r.commandResultMu.RLock()
	defer r.commandResultMu.RUnlock()
	return r.scanCommand(r.db.QueryRowContext(ctx, `
		SELECT command_id, project_id, session_id, device_id, command_type, runtime_snapshot_id, issued_at_us, deadline_at_us,
		       issuer, correlation_id, causation_id, priority, idempotency_key, payload_json, status,
		       result_json, completed_at_us
		FROM stage_device_commands WHERE device_id = ? AND idempotency_key = ?
	`, deviceID, key))
}

func (r *Repository) recordCommandEvent(ctx context.Context, command DeviceCommand, eventType string) error {
	if r.events == nil {
		return nil
	}
	payload, err := json.Marshal(map[string]any{
		"command_id":   command.Envelope.CommandID,
		"device_id":    command.DeviceID,
		"command_type": command.Envelope.CommandType,
		"status":       command.Status,
		"result":       command.Result,
	})
	if err != nil {
		return err
	}
	priority := command.Envelope.Priority
	if priority == "" {
		priority = "P1"
	}
	var sessionID *string
	if command.SessionID != "" {
		value := command.SessionID
		sessionID = &value
	}
	_, err = r.events.AppendEvent(ctx, sessionID, contracts.EventEnvelope{
		EventType:         eventType,
		SchemaVersion:     contracts.SchemaVersion1,
		Source:            "hub.stage_device_runtime",
		ProjectID:         command.Envelope.ProjectID,
		RuntimeSnapshotID: command.Envelope.RuntimeSnapshotID,
		CorrelationID:     command.Envelope.CorrelationID,
		CausationID:       command.Envelope.CausationID,
		Priority:          priority,
		TraceContext:      json.RawMessage(`{}`),
		Payload:           payload,
	})
	return err
}

func commandResultEventType(status contracts.CommandStatus) string {
	switch status {
	case contracts.CommandCompleted:
		return "stage_device.command.completed"
	case contracts.CommandTimedOut:
		return "stage_device.command.timed_out"
	case contracts.CommandCancelled:
		return "stage_device.command.cancelled"
	case contracts.CommandRejected:
		return "stage_device.command.rejected"
	default:
		return "stage_device.command.failed"
	}
}

type scanner interface {
	Scan(dest ...any) error
}

func (r *Repository) scanCommand(row scanner) (DeviceCommand, error) {
	var command DeviceCommand
	var issuedUS int64
	var session sql.NullString
	var deadline, completed sql.NullInt64
	var payload string
	var result sql.NullString
	err := row.Scan(&command.Envelope.CommandID, &command.Envelope.ProjectID, &session, &command.DeviceID,
		&command.Envelope.CommandType, &command.Envelope.RuntimeSnapshotID, &issuedUS, &deadline, &command.Envelope.Issuer,
		&command.Envelope.CorrelationID, &command.Envelope.CausationID, &command.Envelope.Priority,
		&command.Envelope.IdempotencyKey, &payload, &command.Status, &result, &completed)
	if err != nil {
		return DeviceCommand{}, err
	}
	command.Envelope.SchemaVersion = contracts.SchemaVersion1
	command.Envelope.IssuedAt = time.UnixMicro(issuedUS).UTC()
	command.Envelope.Payload = json.RawMessage(payload)
	if session.Valid {
		command.SessionID = session.String
	}
	if deadline.Valid {
		value := time.UnixMicro(deadline.Int64).UTC()
		command.Envelope.DeadlineAt = &value
	}
	if result.Valid {
		command.Result = json.RawMessage(result.String)
	}
	if completed.Valid {
		value := time.UnixMicro(completed.Int64).UTC()
		command.CompletedAt = &value
	}
	return command, nil
}
