-- +goose Up
-- Schema 46 added an explicit audited path from legacy v1 Lighting authority
-- directly into Hub-owned v2 BLOCKED state. Schema 34's epoch-ACK insert guard
-- only recognized BLOCKED assignments produced by the ordinary software-transfer
-- audit, so a freshly migrated legacy node could reconnect, receive BLOCKED and
-- report all-zero output, but SQLite would reject the ACK and the Hub would
-- close the WebSocket.
--
-- Accept either canonical audited route into the exact current BLOCKED epoch:
--   1. committed software transfer; or
--   2. one-way audited legacy v1 -> v2 migration.
-- Neither route grants ACTIVE/snapshot/command authority.
DROP TRIGGER stage_device_epoch_ack_insert_guard;

-- +goose StatementBegin
CREATE TRIGGER stage_device_epoch_ack_insert_guard
BEFORE INSERT ON stage_device_epoch_acks
WHEN (
    NOT EXISTS (
        SELECT 1 FROM stage_device_assignments a
        JOIN stage_devices d ON d.device_id = a.device_id
        JOIN stage_device_assignment_transfers t
          ON t.device_id = a.device_id AND t.to_epoch = a.assignment_epoch
        JOIN stage_device_transfer_intents i ON i.transfer_id = t.transfer_id
        WHERE a.device_id = NEW.device_id
          AND a.assignment_epoch = NEW.assignment_epoch
          AND a.project_id = NEW.project_id
          AND a.assignment_state = 'BLOCKED'
          AND a.runtime_snapshot_id = ''
          AND d.protocol_version = 'stagecore.device/2'
          AND d.enabled = 1
          AND d.project_id IS NULL
          AND t.to_project_id = NEW.project_id
          AND t.next_state = 'BLOCKED'
          AND i.status = 'COMMITTED'
          AND i.connection_generation = t.connection_generation
    )
    AND NOT EXISTS (
        SELECT 1 FROM stage_device_assignments a
        JOIN stage_devices d ON d.device_id = a.device_id
        JOIN stage_device_legacy_v2_migrations m
          ON m.device_id = a.device_id AND m.to_epoch = a.assignment_epoch
        WHERE a.device_id = NEW.device_id
          AND a.assignment_epoch = NEW.assignment_epoch
          AND a.project_id = NEW.project_id
          AND a.assignment_state = 'BLOCKED'
          AND a.runtime_snapshot_id = ''
          AND d.protocol_version = 'stagecore.device/2'
          AND d.enabled = 1
          AND d.project_id IS NULL
          AND m.project_id = NEW.project_id
          AND m.from_protocol = 'stagecore.device/1'
          AND m.to_protocol = 'stagecore.device/2'
          AND m.next_state = 'BLOCKED'
    )
)
OR EXISTS (
    SELECT 1 FROM stage_device_commands
    WHERE device_id = NEW.device_id AND status = 'ACCEPTED'
)
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_EPOCH_ACK_FENCED');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER stage_device_epoch_ack_insert_guard;

-- Restore schema-34 behavior.
-- +goose StatementBegin
CREATE TRIGGER stage_device_epoch_ack_insert_guard
BEFORE INSERT ON stage_device_epoch_acks
WHEN NOT EXISTS (
    SELECT 1 FROM stage_device_assignments a
    JOIN stage_devices d ON d.device_id = a.device_id
    JOIN stage_device_assignment_transfers t
      ON t.device_id = a.device_id AND t.to_epoch = a.assignment_epoch
    JOIN stage_device_transfer_intents i ON i.transfer_id = t.transfer_id
    WHERE a.device_id = NEW.device_id
      AND a.assignment_epoch = NEW.assignment_epoch
      AND a.project_id = NEW.project_id
      AND a.assignment_state = 'BLOCKED'
      AND a.runtime_snapshot_id = ''
      AND d.protocol_version = 'stagecore.device/2'
      AND d.enabled = 1
      AND d.project_id IS NULL
      AND t.to_project_id = NEW.project_id
      AND t.next_state = 'BLOCKED'
      AND i.status = 'COMMITTED'
      AND i.connection_generation = t.connection_generation
)
OR EXISTS (
    SELECT 1 FROM stage_device_commands
    WHERE device_id = NEW.device_id AND status = 'ACCEPTED'
)
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_EPOCH_ACK_FENCED');
END;
-- +goose StatementEnd
