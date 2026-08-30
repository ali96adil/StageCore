-- +goose Up
-- F-028 capture v1 records raw cue timing in the canonical event journal.
-- It deliberately does not create a second analytics/timing table.
-- Historical executions are not backfilled with invented path semantics.
-- Previous Cue identity follows insertion order rather than wall-clock order so
-- a clock regression remains visible as a signed negative interval.
-- +goose StatementBegin
CREATE TRIGGER f028_capture_cue_timing
AFTER INSERT ON cue_executions
WHEN EXISTS (
    SELECT 1
    FROM sessions se
    WHERE se.session_id = NEW.session_id
      AND se.session_type IN ('REHEARSAL', 'SHOW')
)
BEGIN
    INSERT INTO event_records (
        event_id, session_id, event_type, schema_version, occurred_at_us,
        observed_at_us, source_ref, project_id, runtime_snapshot_id,
        correlation_id, causation_id, priority, trace_context_json, payload_json
    )
    SELECT
        lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-' ||
        lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(2))) || '-' ||
        lower(hex(randomblob(6))),
        NEW.session_id,
        'cue.timing_observed',
        1,
        NEW.started_at_us,
        NEW.started_at_us,
        'hub.timing_capture',
        se.project_id,
        se.runtime_snapshot_id,
        NEW.correlation_id,
        (
            SELECT cr.command_id
            FROM command_records cr
            WHERE cr.correlation_id = NEW.correlation_id
              AND cr.project_id = se.project_id
              AND cr.runtime_snapshot_id = se.runtime_snapshot_id
              AND cr.command_type = 'cue.go'
            ORDER BY cr.created_at_us DESC, cr.command_id DESC
            LIMIT 1
        ),
        'P2',
        '{}',
        json_object(
            'capture_version', 1,
            'quality', 'RAW_UNASSESSED',
            'session_type', se.session_type,
            'session_started_at_us', se.started_at_us,
            'cue_execution_id', NEW.cue_execution_id,
            'cue_id', NEW.cue_id,
            'cue_started_at_us', NEW.started_at_us,
            'trigger_source', NEW.trigger_source,
            'manual_override', json(CASE WHEN NEW.manual_override = 1 THEN 'true' ELSE 'false' END),
            'request_issued_at_us', (
                SELECT cr.issued_at_us
                FROM command_records cr
                WHERE cr.correlation_id = NEW.correlation_id
                  AND cr.project_id = se.project_id
                  AND cr.runtime_snapshot_id = se.runtime_snapshot_id
                  AND cr.command_type = 'cue.go'
                ORDER BY cr.created_at_us DESC, cr.command_id DESC
                LIMIT 1
            ),
            'request_to_start_us', CASE
                WHEN (
                    SELECT cr.issued_at_us
                    FROM command_records cr
                    WHERE cr.correlation_id = NEW.correlation_id
                      AND cr.project_id = se.project_id
                      AND cr.runtime_snapshot_id = se.runtime_snapshot_id
                      AND cr.command_type = 'cue.go'
                    ORDER BY cr.created_at_us DESC, cr.command_id DESC
                    LIMIT 1
                ) IS NULL THEN NULL
                ELSE NEW.started_at_us - (
                    SELECT cr.issued_at_us
                    FROM command_records cr
                    WHERE cr.correlation_id = NEW.correlation_id
                      AND cr.project_id = se.project_id
                      AND cr.runtime_snapshot_id = se.runtime_snapshot_id
                      AND cr.command_type = 'cue.go'
                    ORDER BY cr.created_at_us DESC, cr.command_id DESC
                    LIMIT 1
                )
            END,
            'session_elapsed_us', NEW.started_at_us - se.started_at_us,
            'previous_cue_execution_id', (
                SELECT previous.cue_execution_id
                FROM cue_executions previous
                WHERE previous.session_id = NEW.session_id
                  AND previous.rowid < NEW.rowid
                ORDER BY previous.rowid DESC
                LIMIT 1
            ),
            'previous_cue_id', (
                SELECT previous.cue_id
                FROM cue_executions previous
                WHERE previous.session_id = NEW.session_id
                  AND previous.rowid < NEW.rowid
                ORDER BY previous.rowid DESC
                LIMIT 1
            ),
            'previous_cue_started_at_us', (
                SELECT previous.started_at_us
                FROM cue_executions previous
                WHERE previous.session_id = NEW.session_id
                  AND previous.rowid < NEW.rowid
                ORDER BY previous.rowid DESC
                LIMIT 1
            ),
            'cue_to_cue_elapsed_us', CASE
                WHEN (
                    SELECT previous.started_at_us
                    FROM cue_executions previous
                    WHERE previous.session_id = NEW.session_id
                      AND previous.rowid < NEW.rowid
                    ORDER BY previous.rowid DESC
                    LIMIT 1
                ) IS NULL THEN NULL
                ELSE NEW.started_at_us - (
                    SELECT previous.started_at_us
                    FROM cue_executions previous
                    WHERE previous.session_id = NEW.session_id
                      AND previous.rowid < NEW.rowid
                    ORDER BY previous.rowid DESC
                    LIMIT 1
                )
            END,
            'path', json_object(
                'kind', CASE
                    WHEN se.current_cue_id IS NULL THEN
                        CASE
                            WHEN EXISTS (
                                SELECT 1
                                FROM cues before_selected
                                WHERE before_selected.revision_id = rs.revision_id
                                  AND before_selected.enabled = 1
                                  AND (
                                      before_selected.order_index < selected.order_index
                                      OR (
                                          before_selected.order_index = selected.order_index
                                          AND before_selected.cue_id < selected.cue_id
                                      )
                                  )
                            ) THEN 'START_AT_CUE'
                            ELSE 'START'
                        END
                    WHEN se.current_cue_id = NEW.cue_id THEN 'REPEAT'
                    WHEN EXISTS (
                        SELECT 1
                        FROM cues current_cue
                        WHERE current_cue.cue_id = se.current_cue_id
                          AND current_cue.revision_id = rs.revision_id
                          AND (
                              selected.order_index < current_cue.order_index
                              OR (
                                  selected.order_index = current_cue.order_index
                                  AND selected.cue_id < current_cue.cue_id
                              )
                          )
                    ) THEN 'BACK_JUMP'
                    WHEN EXISTS (
                        SELECT 1
                        FROM cues current_cue
                        JOIN cues skipped ON skipped.revision_id = current_cue.revision_id
                        WHERE current_cue.cue_id = se.current_cue_id
                          AND current_cue.revision_id = rs.revision_id
                          AND skipped.enabled = 1
                          AND (
                              skipped.order_index > current_cue.order_index
                              OR (
                                  skipped.order_index = current_cue.order_index
                                  AND skipped.cue_id > current_cue.cue_id
                              )
                          )
                          AND (
                              skipped.order_index < selected.order_index
                              OR (
                                  skipped.order_index = selected.order_index
                                  AND skipped.cue_id < selected.cue_id
                              )
                          )
                    ) THEN 'FORWARD_JUMP'
                    ELSE 'NEXT'
                END,
                'from_cue_id', se.current_cue_id,
                'to_cue_id', NEW.cue_id,
                'skipped_cue_ids', json(
                    CASE
                        WHEN se.current_cue_id IS NULL THEN COALESCE((
                            SELECT json_group_array(ordered.cue_id)
                            FROM (
                                SELECT skipped.cue_id
                                FROM cues skipped
                                WHERE skipped.revision_id = rs.revision_id
                                  AND skipped.enabled = 1
                                  AND (
                                      skipped.order_index < selected.order_index
                                      OR (
                                          skipped.order_index = selected.order_index
                                          AND skipped.cue_id < selected.cue_id
                                      )
                                  )
                                ORDER BY skipped.order_index, skipped.cue_id
                            ) ordered
                        ), '[]')
                        WHEN EXISTS (
                            SELECT 1
                            FROM cues current_cue
                            WHERE current_cue.cue_id = se.current_cue_id
                              AND current_cue.revision_id = rs.revision_id
                              AND (
                                  selected.order_index > current_cue.order_index
                                  OR (
                                      selected.order_index = current_cue.order_index
                                      AND selected.cue_id > current_cue.cue_id
                                  )
                              )
                        ) THEN COALESCE((
                            SELECT json_group_array(ordered.cue_id)
                            FROM (
                                SELECT skipped.cue_id
                                FROM cues current_cue
                                JOIN cues skipped ON skipped.revision_id = current_cue.revision_id
                                WHERE current_cue.cue_id = se.current_cue_id
                                  AND current_cue.revision_id = rs.revision_id
                                  AND skipped.enabled = 1
                                  AND (
                                      skipped.order_index > current_cue.order_index
                                      OR (
                                          skipped.order_index = current_cue.order_index
                                          AND skipped.cue_id > current_cue.cue_id
                                      )
                                  )
                                  AND (
                                      skipped.order_index < selected.order_index
                                      OR (
                                          skipped.order_index = selected.order_index
                                          AND skipped.cue_id < selected.cue_id
                                      )
                                  )
                                ORDER BY skipped.order_index, skipped.cue_id
                            ) ordered
                        ), '[]')
                        ELSE '[]'
                    END
                )
            ),
            'clock', json_object(
                'basis', 'HUB_UTC_WALL',
                'health', 'UNASSESSED',
                'interval_scope', 'SINGLE_HUB',
                'request_basis', CASE
                    WHEN (
                        SELECT cr.issued_at_us
                        FROM command_records cr
                        WHERE cr.correlation_id = NEW.correlation_id
                          AND cr.project_id = se.project_id
                          AND cr.runtime_snapshot_id = se.runtime_snapshot_id
                          AND cr.command_type = 'cue.go'
                        ORDER BY cr.created_at_us DESC, cr.command_id DESC
                        LIMIT 1
                    ) IS NULL THEN 'UNAVAILABLE'
                    ELSE 'COMMAND_ENVELOPE_UTC'
                END
            )
        )
    FROM sessions se
    JOIN runtime_snapshots rs ON rs.runtime_snapshot_id = se.runtime_snapshot_id
    JOIN cues selected ON selected.revision_id = rs.revision_id
                      AND selected.cue_id = NEW.cue_id
    WHERE se.session_id = NEW.session_id;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS f028_capture_cue_timing;
