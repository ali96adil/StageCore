-- +goose Up
-- Reserve one globally monotonic generation for each StageLaser command,
-- serialized by SQLite. A single durable Hub counter never rewinds when
-- projects/snapshots change or when StageCore restarts; devices still enforce
-- their own persistent receive-side watermark.
-- Limit to exact integers representable in both Go/SQLite and cJSON double.
CREATE TABLE stage_device_output_control_sequence (
    singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
    generation INTEGER NOT NULL CHECK(generation >= 0 AND generation <= 9007199254740991)
);
INSERT INTO stage_device_output_control_sequence (singleton, generation)
VALUES (1, 0);

-- +goose StatementBegin
CREATE TRIGGER stage_device_output_control_monotonic
BEFORE UPDATE ON stage_device_output_control_sequence
WHEN NEW.singleton <> OLD.singleton
  OR NEW.generation <> OLD.generation + 1
  OR OLD.generation >= 9007199254740991
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_OUTPUT_CONTROL_GENERATION_REWIND');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER stage_device_output_control_no_delete
BEFORE DELETE ON stage_device_output_control_sequence
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_OUTPUT_CONTROL_GENERATION_DELETE');
END;
-- +goose StatementEnd

ALTER TABLE stage_device_commands
ADD COLUMN control_generation INTEGER NOT NULL DEFAULT 0
    CHECK(control_generation >= 0 AND control_generation <= 9007199254740991);

-- +goose Down
ALTER TABLE stage_device_commands DROP COLUMN control_generation;
DROP TRIGGER IF EXISTS stage_device_output_control_no_delete;
DROP TRIGGER IF EXISTS stage_device_output_control_monotonic;
DROP TABLE IF EXISTS stage_device_output_control_sequence;
