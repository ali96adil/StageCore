package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

func (s *Store) CreateRuntimeSnapshot(ctx context.Context, revisionID, createdBy, contentHash string, manifest json.RawMessage) (domain.RuntimeSnapshot, error) {
	if strings.TrimSpace(revisionID) == "" || len(contentHash) != 64 {
		return domain.RuntimeSnapshot{}, fmt.Errorf("%w: revision and SHA-256 content hash are required", domain.ErrInvalidInput)
	}
	manifestJSON, err := normalizeJSON(manifest, "{}")
	if err != nil {
		return domain.RuntimeSnapshot{}, fmt.Errorf("snapshot manifest: %w", err)
	}
	snapshotID, err := stageid.New()
	if err != nil {
		return domain.RuntimeSnapshot{}, err
	}
	now := s.clock.Now().UTC()
	nowUS := clock.UnixMicros(now)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.RuntimeSnapshot{}, fmt.Errorf("begin create runtime snapshot: %w", err)
	}
	defer tx.Rollback()

	var projectID, status string
	if err := tx.QueryRowContext(ctx, `SELECT project_id, status FROM project_revisions WHERE revision_id = ?`, revisionID).Scan(&projectID, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.RuntimeSnapshot{}, domain.ErrNotFound
		}
		return domain.RuntimeSnapshot{}, fmt.Errorf("read snapshot revision: %w", err)
	}
	if domain.RevisionStatus(status) != domain.RevisionValidated {
		return domain.RuntimeSnapshot{}, fmt.Errorf("%w: runtime snapshot requires VALIDATED revision", domain.ErrConflict)
	}

	var version int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(snapshot_version), 0) + 1 FROM runtime_snapshots WHERE project_id = ?`, projectID).Scan(&version); err != nil {
		return domain.RuntimeSnapshot{}, fmt.Errorf("next snapshot version: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO runtime_snapshots (
			runtime_snapshot_id, project_id, revision_id, snapshot_version, created_at_us, created_by, content_hash, manifest_json, status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'PUBLISHED')`,
		snapshotID, projectID, revisionID, version, nowUS, createdBy, contentHash, manifestJSON); err != nil {
		return domain.RuntimeSnapshot{}, fmt.Errorf("insert runtime snapshot: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.RuntimeSnapshot{}, fmt.Errorf("commit runtime snapshot: %w", err)
	}
	return domain.RuntimeSnapshot{
		ID:              snapshotID,
		ProjectID:       projectID,
		RevisionID:      revisionID,
		SnapshotVersion: version,
		CreatedAt:       now,
		CreatedBy:       createdBy,
		ContentHash:     contentHash,
		Manifest:        json.RawMessage(manifestJSON),
		Status:          domain.SnapshotPublished,
	}, nil
}

func (s *Store) GetRuntimeSnapshot(ctx context.Context, snapshotID string) (domain.RuntimeSnapshot, error) {
	var snapshot domain.RuntimeSnapshot
	var createdUS int64
	var manifest, status string
	err := s.db.QueryRowContext(ctx, `
		SELECT runtime_snapshot_id, project_id, revision_id, snapshot_version, created_at_us, created_by, content_hash, manifest_json, status
		FROM runtime_snapshots WHERE runtime_snapshot_id = ?`, snapshotID).Scan(
		&snapshot.ID, &snapshot.ProjectID, &snapshot.RevisionID, &snapshot.SnapshotVersion, &createdUS,
		&snapshot.CreatedBy, &snapshot.ContentHash, &manifest, &status,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.RuntimeSnapshot{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.RuntimeSnapshot{}, fmt.Errorf("get runtime snapshot: %w", err)
	}
	snapshot.CreatedAt = clock.FromUnixMicros(createdUS)
	snapshot.Manifest = json.RawMessage(manifest)
	snapshot.Status = domain.RuntimeSnapshotStatus(status)
	return snapshot, nil
}

func (s *Store) CreateSession(ctx context.Context, snapshotID string, sessionType domain.SessionType, name string) (domain.Session, error) {
	if sessionType != domain.SessionSimulation && sessionType != domain.SessionRehearsal && sessionType != domain.SessionShow {
		return domain.Session{}, fmt.Errorf("%w: unsupported session type", domain.ErrInvalidInput)
	}
	var projectID, snapshotStatus string
	if err := s.db.QueryRowContext(ctx, `SELECT project_id, status FROM runtime_snapshots WHERE runtime_snapshot_id = ?`, snapshotID).Scan(&projectID, &snapshotStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Session{}, domain.ErrNotFound
		}
		return domain.Session{}, fmt.Errorf("read session snapshot: %w", err)
	}
	if domain.RuntimeSnapshotStatus(snapshotStatus) != domain.SnapshotPublished {
		return domain.Session{}, fmt.Errorf("%w: session requires PUBLISHED snapshot", domain.ErrConflict)
	}
	sessionID, err := stageid.New()
	if err != nil {
		return domain.Session{}, err
	}
	now := s.clock.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (session_id, project_id, runtime_snapshot_id, session_type, name, started_at_us, status)
		VALUES (?, ?, ?, ?, ?, ?, 'ACTIVE')`, sessionID, projectID, snapshotID, sessionType, name, clock.UnixMicros(now)); err != nil {
		return domain.Session{}, fmt.Errorf("insert session: %w", err)
	}
	return domain.Session{
		ID:                sessionID,
		ProjectID:         projectID,
		RuntimeSnapshotID: snapshotID,
		Type:              sessionType,
		Name:              name,
		StartedAt:         now,
		Status:            domain.SessionActive,
	}, nil
}

func (s *Store) GetSession(ctx context.Context, sessionID string) (domain.Session, error) {
	var session domain.Session
	var sessionType, status string
	var startedUS int64
	var endedUS sql.NullInt64
	var current sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT session_id, project_id, runtime_snapshot_id, session_type, name, started_at_us, ended_at_us, status, current_cue_id
		FROM sessions WHERE session_id = ?`, sessionID).Scan(
		&session.ID, &session.ProjectID, &session.RuntimeSnapshotID, &sessionType, &session.Name,
		&startedUS, &endedUS, &status, &current,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Session{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Session{}, fmt.Errorf("get session: %w", err)
	}
	session.Type = domain.SessionType(sessionType)
	session.Status = domain.SessionStatus(status)
	session.StartedAt = clock.FromUnixMicros(startedUS)
	if endedUS.Valid {
		t := clock.FromUnixMicros(endedUS.Int64)
		session.EndedAt = &t
	}
	if current.Valid {
		value := current.String
		session.CurrentCueID = &value
	}
	return session, nil
}

func (s *Store) SetSessionCurrentCue(ctx context.Context, sessionID, cueID string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE sessions
		SET current_cue_id = ?
		WHERE session_id = ?
		  AND status = 'ACTIVE'
		  AND EXISTS (
			SELECT 1
			FROM runtime_snapshots rs
			JOIN cues c ON c.revision_id = rs.revision_id
			WHERE rs.runtime_snapshot_id = sessions.runtime_snapshot_id AND c.cue_id = ?
		  )`, cueID, sessionID, cueID)
	if err != nil {
		return fmt.Errorf("set session current cue: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("session current cue rows affected: %w", err)
	}
	if affected != 1 {
		return domain.ErrConflict
	}
	return nil
}

func (s *Store) CreateCueExecution(ctx context.Context, sessionID, cueID, correlationID, triggerSource string) (domain.CueExecution, error) {
	executionID, err := stageid.New()
	if err != nil {
		return domain.CueExecution{}, err
	}
	now := s.clock.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO cue_executions (
			cue_execution_id, session_id, cue_id, correlation_id, trigger_source, started_at_us, result, manual_override
		) VALUES (?, ?, ?, ?, ?, ?, 'RUNNING', 0)`,
		executionID, sessionID, cueID, correlationID, triggerSource, clock.UnixMicros(now)); err != nil {
		return domain.CueExecution{}, fmt.Errorf("insert cue execution: %w", err)
	}
	return domain.CueExecution{ID: executionID, SessionID: sessionID, CueID: cueID, CorrelationID: correlationID, TriggerSource: triggerSource, StartedAt: now, Result: domain.ExecutionRunning}, nil
}

func (s *Store) FinishCueExecution(ctx context.Context, executionID string, result domain.ExecutionResult) error {
	if result == domain.ExecutionRunning {
		return fmt.Errorf("%w: terminal cue result required", domain.ErrInvalidInput)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE cue_executions SET completed_at_us = ?, result = ? WHERE cue_execution_id = ? AND result = 'RUNNING'`, clock.UnixMicros(s.clock.Now()), result, executionID)
	if err != nil {
		return fmt.Errorf("finish cue execution: %w", err)
	}
	return requireOneRow(res)
}

func (s *Store) CreateActionExecution(ctx context.Context, cueExecutionID, actionID string) (domain.ActionExecution, error) {
	executionID, err := stageid.New()
	if err != nil {
		return domain.ActionExecution{}, err
	}
	now := s.clock.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO action_executions (action_execution_id, cue_execution_id, action_id, started_at_us, result) VALUES (?, ?, ?, ?, 'RUNNING')`, executionID, cueExecutionID, actionID, clock.UnixMicros(now)); err != nil {
		return domain.ActionExecution{}, fmt.Errorf("insert action execution: %w", err)
	}
	return domain.ActionExecution{ID: executionID, CueExecutionID: cueExecutionID, ActionID: actionID, StartedAt: now, Result: domain.ExecutionRunning}, nil
}

func (s *Store) FinishActionExecution(ctx context.Context, executionID string, result domain.ExecutionResult, latencyMS int64, responseSummary string, errorCode *string) error {
	if result == domain.ExecutionRunning || latencyMS < 0 {
		return fmt.Errorf("%w: terminal action result and non-negative latency required", domain.ErrInvalidInput)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE action_executions SET completed_at_us = ?, result = ?, latency_ms = ?, response_summary = ?, error_code = ? WHERE action_execution_id = ? AND result = 'RUNNING'`, clock.UnixMicros(s.clock.Now()), result, latencyMS, responseSummary, errorCode, executionID)
	if err != nil {
		return fmt.Errorf("finish action execution: %w", err)
	}
	return requireOneRow(res)
}

func (s *Store) ListCueExecutions(ctx context.Context, sessionID string) ([]domain.CueExecution, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT cue_execution_id, session_id, cue_id, correlation_id, trigger_source, started_at_us, completed_at_us, result, manual_override FROM cue_executions WHERE session_id = ? ORDER BY started_at_us, cue_execution_id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list cue executions: %w", err)
	}
	defer rows.Close()
	var out []domain.CueExecution
	for rows.Next() {
		var item domain.CueExecution
		var startedUS int64
		var completedUS sql.NullInt64
		var result string
		var manual int
		if err := rows.Scan(&item.ID, &item.SessionID, &item.CueID, &item.CorrelationID, &item.TriggerSource, &startedUS, &completedUS, &result, &manual); err != nil {
			return nil, fmt.Errorf("scan cue execution: %w", err)
		}
		item.StartedAt = clock.FromUnixMicros(startedUS)
		if completedUS.Valid {
			t := clock.FromUnixMicros(completedUS.Int64)
			item.CompletedAt = &t
		}
		item.Result = domain.ExecutionResult(result)
		item.ManualOverride = manual == 1
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListActionExecutions(ctx context.Context, cueExecutionID string) ([]domain.ActionExecution, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT action_execution_id, cue_execution_id, action_id, started_at_us, completed_at_us, result, latency_ms, response_summary, error_code FROM action_executions WHERE cue_execution_id = ? ORDER BY started_at_us, action_execution_id`, cueExecutionID)
	if err != nil {
		return nil, fmt.Errorf("list action executions: %w", err)
	}
	defer rows.Close()
	var out []domain.ActionExecution
	for rows.Next() {
		var item domain.ActionExecution
		var startedUS int64
		var completedUS, latency sql.NullInt64
		var errorCode sql.NullString
		var result string
		if err := rows.Scan(&item.ID, &item.CueExecutionID, &item.ActionID, &startedUS, &completedUS, &result, &latency, &item.ResponseSummary, &errorCode); err != nil {
			return nil, fmt.Errorf("scan action execution: %w", err)
		}
		item.StartedAt = clock.FromUnixMicros(startedUS)
		if completedUS.Valid {
			t := clock.FromUnixMicros(completedUS.Int64)
			item.CompletedAt = &t
		}
		if latency.Valid {
			v := latency.Int64
			item.LatencyMS = &v
		}
		if errorCode.Valid {
			v := errorCode.String
			item.ErrorCode = &v
		}
		item.Result = domain.ExecutionResult(result)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) AppendEvent(ctx context.Context, sessionID *string, event contracts.EventEnvelope) (contracts.EventEnvelope, error) {
	if event.EventID == "" {
		var err error
		event.EventID, err = stageid.New()
		if err != nil {
			return contracts.EventEnvelope{}, err
		}
	}
	if event.SchemaVersion == 0 {
		event.SchemaVersion = contracts.SchemaVersion1
	}
	now := s.clock.Now().UTC()
	if event.OccurredAt.IsZero() {
		event.OccurredAt = now
	}
	if event.ObservedAt.IsZero() {
		event.ObservedAt = now
	}
	trace, err := normalizeJSON(event.TraceContext, "{}")
	if err != nil {
		return contracts.EventEnvelope{}, fmt.Errorf("event trace context: %w", err)
	}
	payload, err := normalizeJSON(event.Payload, "{}")
	if err != nil {
		return contracts.EventEnvelope{}, fmt.Errorf("event payload: %w", err)
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO event_records (event_id, session_id, event_type, schema_version, occurred_at_us, observed_at_us, source_ref, project_id, runtime_snapshot_id, correlation_id, causation_id, priority, trace_context_json, payload_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, event.EventID, nullableString(sessionID), event.EventType, event.SchemaVersion, clock.UnixMicros(event.OccurredAt), clock.UnixMicros(event.ObservedAt), event.Source, event.ProjectID, nullableText(event.RuntimeSnapshotID), nullableText(event.CorrelationID), nullableText(event.CausationID), event.Priority, trace, payload)
	if err != nil {
		return contracts.EventEnvelope{}, fmt.Errorf("append event: %w", err)
	}
	sequence, err := result.LastInsertId()
	if err != nil {
		return contracts.EventEnvelope{}, fmt.Errorf("event sequence: %w", err)
	}
	event.Sequence = sequence
	event.TraceContext = json.RawMessage(trace)
	event.Payload = json.RawMessage(payload)
	return event, nil
}

func (s *Store) ListEvents(ctx context.Context, sessionID string) ([]contracts.EventEnvelope, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT event_id, event_type, schema_version, occurred_at_us, observed_at_us, source_ref, project_id, runtime_snapshot_id, correlation_id, causation_id, priority, sequence, trace_context_json, payload_json FROM event_records WHERE session_id = ? ORDER BY sequence`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()
	var out []contracts.EventEnvelope
	for rows.Next() {
		var event contracts.EventEnvelope
		var occurredUS, observedUS int64
		var snapshotID, correlationID, causationID sql.NullString
		var trace, payload string
		if err := rows.Scan(&event.EventID, &event.EventType, &event.SchemaVersion, &occurredUS, &observedUS, &event.Source, &event.ProjectID, &snapshotID, &correlationID, &causationID, &event.Priority, &event.Sequence, &trace, &payload); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		event.OccurredAt = clock.FromUnixMicros(occurredUS)
		event.ObservedAt = clock.FromUnixMicros(observedUS)
		if snapshotID.Valid { event.RuntimeSnapshotID = snapshotID.String }
		if correlationID.Valid { event.CorrelationID = correlationID.String }
		if causationID.Valid { event.CausationID = causationID.String }
		event.TraceContext = json.RawMessage(trace)
		event.Payload = json.RawMessage(payload)
		out = append(out, event)
	}
	return out, rows.Err()
}

func (s *Store) FindCommandRecord(ctx context.Context, command contracts.CommandEnvelope) (domain.CommandRecord, bool, error) {
	record, found, err := s.getCommandRecord(ctx, `SELECT command_id, command_type, schema_version, issued_at_us, project_id, runtime_snapshot_id, idempotency_key, correlation_id, status, result_json, created_at_us, updated_at_us FROM command_records WHERE command_id = ?`, command.CommandID)
	if err != nil || found || command.IdempotencyKey == "" { return record, found, err }
	return s.getCommandRecord(ctx, `SELECT command_id, command_type, schema_version, issued_at_us, project_id, runtime_snapshot_id, idempotency_key, correlation_id, status, result_json, created_at_us, updated_at_us FROM command_records WHERE project_id = ? AND command_type = ? AND idempotency_key = ?`, command.ProjectID, command.CommandType, command.IdempotencyKey)
}

func (s *Store) ReserveCommand(ctx context.Context, command contracts.CommandEnvelope) (domain.CommandRecord, bool, error) {
	if existing, found, err := s.FindCommandRecord(ctx, command); err != nil || found { return existing, false, err }
	now := s.clock.Now().UTC()
	issuedAt := command.IssuedAt.UTC()
	if issuedAt.IsZero() { issuedAt = now }
	correlationID := command.CorrelationID
	if correlationID == "" { correlationID = command.CommandID }
	_, err := s.db.ExecContext(ctx, `INSERT INTO command_records (command_id, command_type, schema_version, issued_at_us, project_id, runtime_snapshot_id, idempotency_key, correlation_id, status, result_json, created_at_us, updated_at_us) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'ACCEPTED', '{}', ?, ?)`, command.CommandID, command.CommandType, command.SchemaVersion, clock.UnixMicros(issuedAt), command.ProjectID, command.RuntimeSnapshotID, command.IdempotencyKey, correlationID, clock.UnixMicros(now), clock.UnixMicros(now))
	if err != nil {
		if existing, found, lookupErr := s.FindCommandRecord(ctx, command); lookupErr == nil && found { return existing, false, nil }
		return domain.CommandRecord{}, false, fmt.Errorf("reserve command: %w", err)
	}
	return domain.CommandRecord{CommandID: command.CommandID, CommandType: command.CommandType, SchemaVersion: command.SchemaVersion, ProjectID: command.ProjectID, SnapshotID: command.RuntimeSnapshotID, IdempotencyKey: command.IdempotencyKey, CorrelationID: correlationID, Status: string(contracts.CommandAccepted), ResultJSON: json.RawMessage(`{}`), IssuedAt: issuedAt, CreatedAt: now, UpdatedAt: now}, true, nil
}

func (s *Store) FinishCommand(ctx context.Context, commandID string, result contracts.CommandResult) error {
	resultJSON, err := json.Marshal(result)
	if err != nil { return fmt.Errorf("marshal command result: %w", err) }
	res, err := s.db.ExecContext(ctx, `UPDATE command_records SET status = ?, result_json = ?, updated_at_us = ? WHERE command_id = ? AND status = 'ACCEPTED'`, result.Status, string(resultJSON), clock.UnixMicros(s.clock.Now()), commandID)
	if err != nil { return fmt.Errorf("finish command: %w", err) }
	return requireOneRow(res)
}

func (s *Store) getCommandRecord(ctx context.Context, query string, args ...any) (domain.CommandRecord, bool, error) {
	var record domain.CommandRecord
	var issuedUS, createdUS, updatedUS int64
	var resultJSON string
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&record.CommandID, &record.CommandType, &record.SchemaVersion, &issuedUS, &record.ProjectID, &record.SnapshotID, &record.IdempotencyKey, &record.CorrelationID, &record.Status, &resultJSON, &createdUS, &updatedUS)
	if errors.Is(err, sql.ErrNoRows) { return domain.CommandRecord{}, false, nil }
	if err != nil { return domain.CommandRecord{}, false, fmt.Errorf("get command record: %w", err) }
	record.ResultJSON = json.RawMessage(resultJSON)
	record.IssuedAt = clock.FromUnixMicros(issuedUS)
	record.CreatedAt = clock.FromUnixMicros(createdUS)
	record.UpdatedAt = clock.FromUnixMicros(updatedUS)
	return record, true, nil
}

func (s *Store) HasRunningCueExecution(ctx context.Context, sessionID string) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cue_executions WHERE session_id = ? AND result = 'RUNNING'`, sessionID).Scan(&count); err != nil { return false, fmt.Errorf("check running cue execution: %w", err) }
	return count > 0, nil
}

func requireOneRow(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil { return fmt.Errorf("rows affected: %w", err) }
	if affected != 1 { return domain.ErrConflict }
	return nil
}

func nullableText(value string) any { if value == "" { return nil }; return value }
func nullableString(value *string) any { if value == nil || *value == "" { return nil }; return *value }

func terminalCommandStatus(status string) bool {
	switch contracts.CommandStatus(status) {
	case contracts.CommandRejected, contracts.CommandCompleted, contracts.CommandFailed, contracts.CommandTimedOut, contracts.CommandCancelled:
		return true
	default:
		return false
	}
}

func decodeCommandResult(record domain.CommandRecord) (contracts.CommandResult, error) {
	if !terminalCommandStatus(record.Status) { return contracts.CommandResult{}, fmt.Errorf("command record is not terminal") }
	var result contracts.CommandResult
	if err := json.Unmarshal(record.ResultJSON, &result); err != nil { return contracts.CommandResult{}, fmt.Errorf("decode command result: %w", err) }
	return result, nil
}

func (s *Store) StoredCommandResult(record domain.CommandRecord) (contracts.CommandResult, bool, error) {
	if !terminalCommandStatus(record.Status) { return contracts.CommandResult{}, false, nil }
	result, err := decodeCommandResult(record)
	return result, true, err
}

func durationMilliseconds(started, completed time.Time) int64 {
	if completed.Before(started) { return 0 }
	return completed.Sub(started).Milliseconds()
}
