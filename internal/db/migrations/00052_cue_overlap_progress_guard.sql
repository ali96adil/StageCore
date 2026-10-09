-- +goose Up
-- A later Cue may complete before an earlier delayed Cue. The old progress
-- trigger used NEW.cue_id unconditionally and could rewind the operator's
-- active Cue and next-Cue pointer after a legitimate overlapping GO/JUMP.
DROP TRIGGER IF EXISTS session_foundation_advance_progress;

-- +goose StatementBegin
CREATE TRIGGER session_foundation_advance_progress
AFTER UPDATE OF result ON cue_executions
WHEN OLD.result = 'RUNNING' AND NEW.result IN ('COMPLETED', 'FAILED', 'TIMED_OUT')
BEGIN
    -- Last completed is an observation, not an operator navigation cursor.
    UPDATE sessions
    SET last_completed_cue_id = CASE
        WHEN NEW.result = 'COMPLETED' THEN NEW.cue_id
        ELSE last_completed_cue_id
    END
    WHERE session_id = NEW.session_id;

    -- Only the Cue still selected by the Operator is allowed to advance
    -- runtime navigation on completion. A prior overlapping Cue cannot
    -- reclaim the current position; this also preserves intentional JUMP.
    -- A NULL current_cue_id retains the legacy terminal-first fallback.
    UPDATE sessions
    SET current_cue_id = NEW.cue_id,
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
    WHERE session_id = NEW.session_id
      AND (current_cue_id = NEW.cue_id OR current_cue_id IS NULL);
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS session_foundation_advance_progress;

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
