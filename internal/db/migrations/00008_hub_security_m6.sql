-- +goose Up
CREATE TABLE hub_security (
    singleton_id INTEGER PRIMARY KEY CHECK (singleton_id = 1),
    hub_id TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    public_key BLOB NOT NULL,
    fingerprint TEXT NOT NULL UNIQUE,
    bootstrap_state TEXT NOT NULL CHECK (bootstrap_state IN ('UNCLAIMED','CLAIMED')),
    created_at_us INTEGER NOT NULL,
    claimed_at_us INTEGER
);

CREATE TABLE hub_setup_codes (
    setup_code_id TEXT PRIMARY KEY,
    code_sha256 BLOB NOT NULL UNIQUE,
    created_at_us INTEGER NOT NULL,
    expires_at_us INTEGER NOT NULL,
    consumed_at_us INTEGER,
    CHECK (expires_at_us > created_at_us)
);
CREATE INDEX hub_setup_codes_active_idx ON hub_setup_codes (expires_at_us, consumed_at_us);

CREATE TABLE local_users (
    user_id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE COLLATE NOCASE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('OWNER','TECHNICIAN','OPERATOR','VIEWER')),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
    created_at_us INTEGER NOT NULL,
    updated_at_us INTEGER NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS local_users;
DROP INDEX IF EXISTS hub_setup_codes_active_idx;
DROP TABLE IF EXISTS hub_setup_codes;
DROP TABLE IF EXISTS hub_security;
