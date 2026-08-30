-- +goose Up
ALTER TABLE sessions ADD COLUMN session_contract_version INTEGER NOT NULL DEFAULT 1 CHECK(session_contract_version > 0);
ALTER TABLE sessions ADD COLUMN lifecycle_state TEXT NOT NULL DEFAULT 'ACTIVE'
    CHECK(lifecycle_state IN ('ACTIVE', 'COMPLETED', 'STOPPED', 'SUSPENDED', 'ABORTED'));
ALTER TABLE sessions ADD COLUMN end_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN start_position_version INTEGER NOT NULL DEFAULT 1 CHECK(start_position_version > 0);
ALTER TABLE sessions ADD COLUMN start_position_kind TEXT NOT NULL DEFAULT 'BEGINNING'
    CHECK(start_position_kind IN ('UNSPECIFIED', 'BEGINNING', 'CUE', 'SCENE', 'RANGE', 'CHECKPOINT'));
ALTER TABLE sessions ADD COLUMN start_cue_id TEXT NULL REFERENCES cues(cue_id) ON DELETE RESTRICT;
ALTER TABLE sessions ADD COLUMN start_position_metadata_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE sessions ADD COLUMN last_completed_cue_id TEXT NULL REFERENCES cues(cue_id) ON DELETE RESTRICT;
ALTER TABLE sessions ADD COLUMN next_cue_id TEXT NULL REFERENCES cues(cue_id) ON DELETE RESTRICT;
ALTER TABLE sessions ADD COLUMN state_truth_version INTEGER NOT NULL DEFAULT 1 CHECK(state_truth_version > 0);
ALTER TABLE sessions ADD COLUMN restoration_status TEXT NOT NULL DEFAULT 'NOT_ASSESSED'
    CHECK(restoration_status IN ('NOT_ASSESSED', 'NOT_REQUIRED', 'RESTORABLE', 'MANUAL_CONFIRMATION_REQUIRED', 'UNAVAILABLE'));
ALTER TABLE sessions ADD COLUMN desired_state_ref TEXT NULL;
ALTER TABLE sessions ADD COLUMN verified_state_ref TEXT NULL;
ALTER TABLE sessions ADD COLUMN manual_confirmation_required INTEGER NOT NULL DEFAULT 0
    CHECK(manual_confirmation_required IN (0, 1));

-- Rows that existed before F-027 must not acquire invented start semantics.
UPDATE sessions
SET start_position_kind = 'UNSPECIFIED',
    lifecycle_state = CASE status
        WHEN 'ACTIVE' THEN 'ACTIVE'
        WHEN 'COMPLETED' THEN 'COMPLETED'
        ELSE 'ABORTED'
    END,
    last_completed_cue_id = (
        SELECT ce.cue_id
        FROM cue_executions ce
        WHERE ce.session_id = sessions.session_id AND ce.result = 'COMPLETED'
        ORDER BY ce.completed_at_us DESC, ce.cue_execution_id DESC
        LIMIT 1
    );

-- New BEGINNING sessions get an explicit logical next cue while preserving
-- current_cue_id as historical/observed progress rather than desired state.
-- +goose StatementBegin
CREATE TRIGGER session_foundation_initialize_beginning
AFTER INSERT ON sessions
WHEN NEW.start_position_kind = 'BEGINNING'
BEGIN
    UPDATE sessions
    SET next_cue_id = (
            SELECT c.cue_id
            FROM runtime_snapshots rs
            JOIN cues c ON c.revision_id = rs.revision_id
            WHERE rs.runtime_snapshot_id = NEW.runtime_snapshot_id
              AND c.enabled = 1
            ORDER BY c.order_index, c.cue_id
            LIMIT 1
        ),
        restoration_status = 'NOT_REQUIRED',
        manual_confirmation_required = 0
    WHERE session_id = NEW.session_id;
END;
-- +goose StatementEnd

-- The existing restart reconciler marks ACTIVE sessions ABORTED after it
-- cancels in-flight work. Classify that legacy coarse transition truthfully:
-- rehearsals become resumable SUSPENDED candidates, while SHOW remains
-- strictly ABORTED. The canonical event journal records the interruption.
-- +goose StatementBegin
CREATE TRIGGER session_foundation_classify_restart_interruption
AFTER UPDATE OF status ON sessions
WHEN OLD.status = 'ACTIVE'
 AND NEW.status = 'ABORTED'
 AND NEW.lifecycle_state = 'ACTIVE'
 AND NEW.end_reason = ''
BEGIN
    UPDATE sessions
    SET lifecycle_state = CASE
            WHEN NEW.session_type = 'REHEARSAL' THEN 'SUSPENDED'
            ELSE 'ABORTED'
        END,
        end_reason = 'HUB_RESTART_INTERRUPTED',
        restoration_status = 'MANUAL_CONFIRMATION_REQUIRED',
        manual_confirmation_required = 1
    WHERE session_id = NEW.session_id;

    INSERT INTO event_records (
        event_id, session_id, event_type, schema_version, occurred_at_us,
        observed_at_us, source_ref, project_id, runtime_snapshot_id,
        correlation_id, causation_id, priority, trace_context_json, payload_json
    ) VALUES (
        lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-' ||
        lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(2))) || '-' ||
        lower(hex(randomblob(6))),
        NEW.session_id,
        CASE
            WHEN NEW.session_type = 'REHEARSAL' THEN 'rehearsal.suspended'
            WHEN NEW.session_type = 'SHOW' THEN 'show.interrupted'
            ELSE 'simulation.interrupted'
        END,
        1, NEW.ended_at_us, NEW.ended_at_us, 'hub.runtime_recovery',
        NEW.project_id, NEW.runtime_snapshot_id, NULL, NULL, 'P1', '{}',
        '{"session_id":"' || NEW.session_id ||
        '","lifecycle_state":"' ||
        CASE WHEN NEW.session_type = 'REHEARSAL' THEN 'SUSPENDED' ELSE 'ABORTED' END ||
        '","reason":"HUB_RESTART_INTERRUPTED"}'
    );
END;
-- +goose StatementEnd

-- Terminal Cue results advance persisted logical progress. CANCELLED is
-- intentionally excluded: an interrupted/stopped Cue remains the candidate
-- that must be reconciled rather than being silently skipped.
-- +goose StatementBegin
CREATE TRIGGER session_foundation_advance_progress
AFTER UPDATE OF result ON cue_executions
WHEN OLD.result = 'RUNNING' AND NEW.result IN ('COMPLETED', 'FAILED', 'TIMED_OUT')
BEGIN
    UPDATE sessions
    SET current_cue_id = NEW.cue_id,
        last_completed_cue_id = CASE
            WHEN NEW.result = 'COMPLETED' THEN NEW.cue_id
            ELSE last_completed_cue_id
        END,
        next_cue_id = (
            SELECT c2.cue_id
            FROM runtime_snapshots rs
            JOIN cues current_cue ON current_cue.revision_id = rs.revision_id
                                 AND current_cue.cue_id = NEW.cue_id
            JOIN cues c2 ON c2.revision_id = rs.revision_id
            WHERE rs.runtime_snapshot_id = sessions.runtime_snapshot_id
              AND c2.enabled = 1
              AND (
                    c2.order_index > current_cue.order_index
                    OR (c2.order_index = current_cue.order_index AND c2.cue_id > current_cue.cue_id)
                  )
            ORDER BY c2.order_index, c2.cue_id
            LIMIT 1
        )
    WHERE session_id = NEW.session_id;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS session_foundation_advance_progress;
DROP TRIGGER IF EXISTS session_foundation_classify_restart_interruption;
DROP TRIGGER IF EXISTS session_foundation_initialize_beginning;
ALTER TABLE sessions DROP COLUMN manual_confirmation_required;
ALTER TABLE sessions DROP COLUMN verified_state_ref;
ALTER TABLE sessions DROP COLUMN desired_state_ref;
ALTER TABLE sessions DROP COLUMN restoration_status;
ALTER TABLE sessions DROP COLUMN state_truth_version;
ALTER TABLE sessions DROP COLUMN next_cue_id;
ALTER TABLE sessions DROP COLUMN last_completed_cue_id;
ALTER TABLE sessions DROP COLUMN start_position_metadata_json;
ALTER TABLE sessions DROP COLUMN start_cue_id;
ALTER TABLE sessions DROP COLUMN start_position_kind;
ALTER TABLE sessions DROP COLUMN start_position_version;
ALTER TABLE sessions DROP COLUMN end_reason;
ALTER TABLE sessions DROP COLUMN lifecycle_state;
ALTER TABLE sessions DROP COLUMN session_contract_version;
