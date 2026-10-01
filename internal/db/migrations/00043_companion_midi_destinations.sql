-- +goose Up
ALTER TABLE companions
ADD COLUMN midi_destinations_json TEXT NOT NULL DEFAULT '[]';

-- +goose Down
-- SQLite cannot safely drop this column on all supported versions; keep it on downgrade.
