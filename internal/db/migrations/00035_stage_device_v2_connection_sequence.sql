-- +goose Up
-- Hub-issued v2 connection generations must never repeat after a Hub restart.
-- Unlike an in-memory counter, this singleton is serialized by SQLite and
-- seeded above every transfer/epoch acknowledgment already stored on disk.
-- This is NOT Project authority or physical DMX proof.
CREATE TABLE stage_device_v2_connection_sequence (
    singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
    generation INTEGER NOT NULL CHECK(generation >= 0 AND generation <= 9223372036854775807)
);

INSERT INTO stage_device_v2_connection_sequence (singleton, generation)
SELECT 1, COALESCE(MAX(generation), 0)
FROM (
    SELECT connection_generation AS generation FROM stage_device_transfer_intents
    UNION ALL
    SELECT connection_generation AS generation FROM stage_device_epoch_acks
);

-- +goose StatementBegin
CREATE TRIGGER stage_device_v2_connection_monotonic
BEFORE UPDATE ON stage_device_v2_connection_sequence
WHEN NEW.singleton != OLD.singleton
  OR NEW.generation != OLD.generation + 1
  OR OLD.generation >= 9223372036854775807
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_CONNECTION_GENERATION_REWIND');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER stage_device_v2_connection_no_delete
BEFORE DELETE ON stage_device_v2_connection_sequence
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_CONNECTION_GENERATION_DELETE');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS stage_device_v2_connection_no_delete;
DROP TRIGGER IF EXISTS stage_device_v2_connection_monotonic;
DROP TABLE IF EXISTS stage_device_v2_connection_sequence;
