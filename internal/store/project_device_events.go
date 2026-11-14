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

const projectDeviceTransportEventLimit = 500

// ListProjectDeviceTransportEvents returns only unscoped, authenticated
// transport transitions for one Project and a bounded time interval. Command
// events are deliberately excluded; a command attempt is not connection
// health evidence.
func (s *Store) ListProjectDeviceTransportEvents(
	ctx context.Context,
	projectID string,
	from, through time.Time,
) ([]contracts.EventEnvelope, bool, error) {
	projectID = strings.TrimSpace(projectID)
	from = from.UTC()
	through = through.UTC()
	if projectID == "" || from.IsZero() || through.IsZero() || through.Before(from) {
		return nil, false, fmt.Errorf("project and valid transport event interval are required")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT event_id, event_type, schema_version, occurred_at_us, observed_at_us,
		       source_ref, project_id, runtime_snapshot_id, correlation_id,
		       causation_id, priority, sequence, trace_context_json, payload_json
		FROM event_records
		WHERE session_id IS NULL AND source_ref = 'hub.stage_device_runtime' AND project_id = ?
		  AND event_type IN (
		      'stage_device.transport.connected',
		      'stage_device.transport.disconnected'
		  )
		  AND occurred_at_us >= ? AND occurred_at_us <= ?
		ORDER BY occurred_at_us, sequence
		LIMIT ?
	`, projectID, clock.UnixMicros(from), clock.UnixMicros(through), projectDeviceTransportEventLimit+1)
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
	truncated := len(events) > projectDeviceTransportEventLimit
	if truncated {
		events = events[:projectDeviceTransportEventLimit]
	}
	return events, truncated, nil
}
