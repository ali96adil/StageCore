-- +goose Up
ALTER TABLE companions
ADD COLUMN midi_destinations_json TEXT NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE companions DROP COLUMN midi_destinations_json;
