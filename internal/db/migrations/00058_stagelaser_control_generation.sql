-- +goose Up
-- StageLaser generation numbers must never restart at zero when upgrading
-- a legacy database that already contains command generations.
-- The column is prepared conditionally by db.migrate after goose.UpTo(57):
-- SQLite does not support ALTER TABLE ADD COLUMN IF NOT EXISTS.
CREATE TABLE stagelaser_control_sequence (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    generation INTEGER NOT NULL CHECK (generation BETWEEN 0 AND 9007199254740991)
);
INSERT INTO stagelaser_control_sequence (id, generation)
    SELECT 1, COALESCE(MAX(control_generation), 0)
    FROM stage_device_commands;

-- Older deployed schemas can have control_generation without its bounded
-- CHECK. Keep the same fail-closed limit for future inserts and updates.
-- +goose StatementBegin
CREATE TRIGGER stage_device_control_generation_insert_guard
BEFORE INSERT ON stage_device_commands
WHEN NEW.control_generation IS NULL
  OR NEW.control_generation < 0
  OR NEW.control_generation > 9007199254740991
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_CONTROL_GENERATION_INVALID');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER stage_device_control_generation_update_guard
BEFORE UPDATE OF control_generation ON stage_device_commands
WHEN NEW.control_generation IS NULL
  OR NEW.control_generation < 0
  OR NEW.control_generation > 9007199254740991
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_CONTROL_GENERATION_INVALID');
END;
-- +goose StatementEnd

-- Fails closed if deployed data contains duplicate positive generations.
CREATE UNIQUE INDEX stage_device_commands_control_generation
    ON stage_device_commands (control_generation) WHERE control_generation > 0;

-- +goose Down
DROP INDEX stage_device_commands_control_generation;
DROP TRIGGER stage_device_control_generation_update_guard;
DROP TRIGGER stage_device_control_generation_insert_guard;
DROP TABLE stagelaser_control_sequence;
-- Intentionally preserve control_generation and its historical data on
-- rollback; removing the column would destroy deployed command history.
