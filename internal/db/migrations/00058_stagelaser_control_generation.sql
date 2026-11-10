-- +goose Up
-- The Hub and cJSON must agree on the exact integer carried on the wire.
CREATE TABLE stagelaser_control_sequence (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    generation INTEGER NOT NULL CHECK (generation BETWEEN 0 AND 9007199254740991)
);
INSERT INTO stagelaser_control_sequence (id, generation) VALUES (1, 0);

ALTER TABLE stage_device_commands ADD COLUMN control_generation
    INTEGER NOT NULL DEFAULT 0 CHECK (control_generation BETWEEN 0 AND 9007199254740991);

CREATE UNIQUE INDEX stage_device_commands_control_generation
    ON stage_device_commands (control_generation) WHERE control_generation > 0;

-- +goose Down
DROP INDEX stage_device_commands_control_generation;
ALTER TABLE stage_device_commands DROP COLUMN control_generation;
DROP TABLE stagelaser_control_sequence;
