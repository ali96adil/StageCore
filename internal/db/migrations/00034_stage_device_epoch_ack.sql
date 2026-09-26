-- +goose Up
-- An authenticated v2 node may report software-confirmed zero for its
-- Hub-committed BLOCKED epoch. This is evidence of a *reported* logical state,
-- not physical measurement and NEVER permission for ACTIVE or new commands.
CREATE TABLE stage_device_epoch_acks (
    device_id TEXT NOT NULL REFERENCES stage_device_identity_registry(device_id) ON DELETE RESTRICT,
    assignment_epoch INTEGER NOT NULL CHECK(assignment_epoch > 1),
    project_id TEXT NOT NULL CHECK(project_id <> ''),
    connection_generation INTEGER NOT NULL CHECK(connection_generation > 0),
    channel_count INTEGER NOT NULL CHECK(channel_count = 12),
    acked_at_us INTEGER NOT NULL CHECK(acked_at_us > 0),
    PRIMARY KEY(device_id, assignment_epoch)
);

-- Do not accept fabricated epoch metadata absent the exact durable audited
-- transfer. Runtime code independently checks the authenticated active socket
-- and complete zero output report; SQL checks the committed Project and epoch.
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

-- Future reconnects may re-ack the same epoch on a fresh authenticated
-- connection. The immutable Project and epoch must never be rewritten.
-- +goose StatementBegin
CREATE TRIGGER stage_device_epoch_ack_update_guard
BEFORE UPDATE ON stage_device_epoch_acks
WHEN NEW.device_id IS NOT OLD.device_id
  OR NEW.assignment_epoch IS NOT OLD.assignment_epoch
  OR NEW.project_id IS NOT OLD.project_id
  OR NEW.channel_count IS NOT OLD.channel_count
  OR NEW.acked_at_us < OLD.acked_at_us
  OR NOT EXISTS (
      SELECT 1 FROM stage_device_assignments a
      WHERE a.device_id = NEW.device_id
        AND a.assignment_epoch = NEW.assignment_epoch
        AND a.project_id = NEW.project_id
        AND a.assignment_state = 'BLOCKED'
        AND a.runtime_snapshot_id = ''
  )
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_EPOCH_ACK_STALE');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS stage_device_epoch_ack_update_guard;
DROP TRIGGER IF EXISTS stage_device_epoch_ack_insert_guard;
DROP TABLE IF EXISTS stage_device_epoch_acks;
