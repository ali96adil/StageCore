-- +goose Up
-- Session Memory reads the sparse, project-scoped transport-transition stream
-- by time window. Keep that query bounded without indexing all runtime events.
CREATE INDEX event_records_project_transport_timeline_idx
    ON event_records(project_id, occurred_at_us, sequence)
    WHERE session_id IS NULL
      AND source_ref = 'hub.stage_device_runtime'
      AND event_type IN (
          'stage_device.transport.connected',
          'stage_device.transport.disconnected'
      );

-- +goose Down
DROP INDEX IF EXISTS event_records_project_transport_timeline_idx;
