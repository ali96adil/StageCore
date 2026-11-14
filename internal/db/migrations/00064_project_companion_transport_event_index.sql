-- +goose Up
-- Keep Companion transport history on the same bounded Project timeline path
-- without indexing unrelated runtime events.
CREATE INDEX event_records_project_companion_transport_timeline_idx
    ON event_records(project_id, occurred_at_us, sequence)
    WHERE session_id IS NULL
      AND source_ref = 'hub.companion_runtime'
      AND event_type IN (
          'companion.transport.connected',
          'companion.transport.disconnected'
      );

-- +goose Down
DROP INDEX IF EXISTS event_records_project_companion_transport_timeline_idx;
