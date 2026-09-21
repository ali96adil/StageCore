-- +goose Up
-- A reservation may become COMMITTED only in the SAME transaction as its
-- canonical v2 assignment audit and epoch CAS. This is still no evidence
-- that the device's physical DMX fixture is dark or that a new epoch is ACKed.
DROP TRIGGER stage_device_transfer_status_guard;

-- +goose StatementBegin
CREATE TRIGGER stage_device_transfer_status_guard
BEFORE UPDATE ON stage_device_transfer_intents
WHEN NOT (
    NEW.transfer_id IS OLD.transfer_id
    AND NEW.device_id IS OLD.device_id
    AND NEW.from_project_id IS OLD.from_project_id
    AND NEW.to_project_id IS OLD.to_project_id
    AND NEW.expected_epoch IS OLD.expected_epoch
    AND NEW.connection_generation IS OLD.connection_generation
    AND NEW.expected_channels IS OLD.expected_channels
    AND NEW.challenge_sha256 IS OLD.challenge_sha256
    AND NEW.requested_by IS OLD.requested_by
    AND NEW.created_at_us IS OLD.created_at_us
    AND NEW.expires_at_us IS OLD.expires_at_us
    AND NEW.updated_at_us >= OLD.updated_at_us
    AND OLD.status = 'PENDING'
    AND (
        NEW.status IN ('EXPIRED', 'CANCELLED')
        OR (
            NEW.status = 'COMMITTED'
            AND NEW.updated_at_us < OLD.expires_at_us
            AND EXISTS (
                SELECT 1 FROM stage_device_assignment_transfers t
                JOIN stage_device_assignments a ON a.device_id = t.device_id
                WHERE t.transfer_id = OLD.transfer_id
                  AND t.device_id = OLD.device_id
                  AND t.actor_id = OLD.requested_by
                  AND t.from_project_id = COALESCE(OLD.from_project_id, '')
                  AND t.to_project_id = COALESCE(OLD.to_project_id, '')
                  AND t.from_epoch = OLD.expected_epoch
                  AND t.to_epoch = OLD.expected_epoch + 1
                  AND t.connection_generation = OLD.connection_generation
                  AND t.challenge_sha256 = OLD.challenge_sha256
                  AND t.next_state IN ('BLOCKED', 'UNASSIGNED')
                  AND a.assignment_epoch = t.to_epoch
                  AND a.project_id IS OLD.to_project_id
                  AND a.assignment_state = t.next_state
                  AND a.runtime_snapshot_id = ''
            )
        )
    )
)
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_TRANSFER_TRANSITION_FENCED');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER stage_device_transfer_status_guard;

-- Restore the original schema-32 fail-closed guard on rollback.
-- +goose StatementBegin
CREATE TRIGGER stage_device_transfer_status_guard
BEFORE UPDATE ON stage_device_transfer_intents
WHEN NOT (
    OLD.status = 'PENDING' AND NEW.status IN ('EXPIRED', 'CANCELLED')
    AND NEW.transfer_id IS OLD.transfer_id
    AND NEW.device_id IS OLD.device_id
    AND NEW.from_project_id IS OLD.from_project_id
    AND NEW.to_project_id IS OLD.to_project_id
    AND NEW.expected_epoch IS OLD.expected_epoch
    AND NEW.connection_generation IS OLD.connection_generation
    AND NEW.expected_channels IS OLD.expected_channels
    AND NEW.challenge_sha256 IS OLD.challenge_sha256
    AND NEW.requested_by IS OLD.requested_by
    AND NEW.created_at_us IS OLD.created_at_us
    AND NEW.expires_at_us IS OLD.expires_at_us
    AND NEW.updated_at_us >= OLD.updated_at_us
)
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_TRANSFER_TRANSITION_FENCED');
END;
-- +goose StatementEnd
