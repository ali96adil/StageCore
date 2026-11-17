package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
)

const projectTransportEventLimit = 500

// ListProjectDeviceTransportEvents is the compatibility query for Stage Device
// transport transitions. Command events are deliberately excluded.
func (s *Store) ListProjectDeviceTransportEvents(
	ctx context.Context,
	projectID string,
	from, through time.Time,
) ([]contracts.EventEnvelope, bool, error) {
	return s.listProjectTransportEvents(ctx, projectID, from, through, false)
}

// ListProjectTransportEvents returns only unscoped, authenticated Hub
// transport transitions for one Project and bounded time interval. A
// transport event is not evidence that the Companion app, an adapter, or a
// physical output is ready.
func (s *Store) ListProjectTransportEvents(
	ctx context.Context,
	projectID string,
	from, through time.Time,
) ([]contracts.EventEnvelope, bool, error) {
	return s.listProjectTransportEvents(ctx, projectID, from, through, true)
}

func (s *Store) listProjectTransportEvents(
	ctx context.Context,
	projectID string,
	from, through time.Time,
	includeCompanion bool,
) ([]contracts.EventEnvelope, bool, error) {
	projectID = strings.TrimSpace(projectID)
	from = from.UTC()
	through = through.UTC()
	if projectID == "" || from.IsZero() || through.IsZero() || through.Before(from) {
		return nil, false, fmt.Errorf("project and valid transport event interval are required")
	}
	const columns = `
		SELECT event_id, event_type, schema_version, occurred_at_us, observed_at_us,
		       source_ref, project_id, runtime_snapshot_id, correlation_id,
		       causation_id, priority, sequence, trace_context_json, payload_json
		FROM event_records`
	deviceFilter := `WHERE session_id IS NULL AND source_ref = 'hub.stage_device_runtime'
		  AND event_type IN ('stage_device.transport.connected', 'stage_device.transport.disconnected')
		  AND project_id = ?
		  AND occurred_at_us >= ? AND occurred_at_us <= ?
	`
	query := columns + " " + deviceFilter + " ORDER BY occurred_at_us, sequence LIMIT ?"
	args := []any{projectID, clock.UnixMicros(from), clock.UnixMicros(through), projectTransportEventLimit + 1}
	if includeCompanion {
		companionFilter := `WHERE session_id IS NULL AND source_ref = 'hub.companion_runtime'
		  AND event_type IN ('companion.transport.connected', 'companion.transport.disconnected')
		  AND project_id = ?
		  AND occurred_at_us >= ? AND occurred_at_us <= ?`
		query = columns + " " + deviceFilter + " UNION ALL " + columns + " " + companionFilter +
			" ORDER BY occurred_at_us, sequence LIMIT ?"
		args = []any{
			projectID, clock.UnixMicros(from), clock.UnixMicros(through),
			projectID, clock.UnixMicros(from), clock.UnixMicros(through), projectTransportEventLimit + 1,
		}
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("list Project transport events: %w", err)
	}
	defer rows.Close()
	events := make([]contracts.EventEnvelope, 0)
	for rows.Next() {
		var event contracts.EventEnvelope
		var occurredUS, observedUS int64
		var snapshotID, correlationID, causationID sql.NullString
		var trace, payload string
		if err := rows.Scan(&event.EventID, &event.EventType, &event.SchemaVersion,
			&occurredUS, &observedUS, &event.Source, &event.ProjectID, &snapshotID,
			&correlationID, &causationID, &event.Priority, &event.Sequence, &trace, &payload); err != nil {
			return nil, false, fmt.Errorf("scan Project transport event: %w", err)
		}
		event.OccurredAt = clock.FromUnixMicros(occurredUS)
		event.ObservedAt = clock.FromUnixMicros(observedUS)
		if snapshotID.Valid {
			event.RuntimeSnapshotID = snapshotID.String
		}
		if correlationID.Valid {
			event.CorrelationID = correlationID.String
		}
		if causationID.Valid {
			event.CausationID = causationID.String
		}
		event.TraceContext = []byte(trace)
		event.Payload = []byte(payload)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	truncated := len(events) > projectTransportEventLimit
	if truncated {
		events = events[:projectTransportEventLimit]
	}
	return events, truncated, nil
}
