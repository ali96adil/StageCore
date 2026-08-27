-- +goose Up
CREATE TABLE secret_records (
    secret_id TEXT PRIMARY KEY,
    logical_name TEXT NOT NULL UNIQUE,
    nonce BLOB NOT NULL,
    ciphertext BLOB NOT NULL,
    created_at_us INTEGER NOT NULL,
    updated_at_us INTEGER NOT NULL,
    CHECK (length(logical_name) BETWEEN 1 AND 128),
    CHECK (length(nonce) >= 12),
    CHECK (length(ciphertext) > 0)
);
CREATE INDEX secret_records_updated_idx ON secret_records (updated_at_us, secret_id);

-- +goose Down
DROP INDEX IF EXISTS secret_records_updated_idx;
DROP TABLE IF EXISTS secret_records;
