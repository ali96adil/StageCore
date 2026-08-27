-- +goose Up
CREATE TABLE browser_sessions (
    session_id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES local_users(user_id) ON DELETE CASCADE,
    token_sha256 BLOB NOT NULL UNIQUE,
    csrf_sha256 BLOB NOT NULL,
    issued_at_us INTEGER NOT NULL,
    expires_at_us INTEGER NOT NULL,
    revoked_at_us INTEGER,
    last_seen_at_us INTEGER NOT NULL,
    CHECK (expires_at_us > issued_at_us)
);
CREATE INDEX browser_sessions_user_active_idx ON browser_sessions (user_id, expires_at_us, revoked_at_us);

CREATE TABLE auth_login_state (
    username TEXT NOT NULL COLLATE NOCASE,
    remote_key TEXT NOT NULL,
    failed_count INTEGER NOT NULL DEFAULT 0 CHECK (failed_count >= 0),
    blocked_until_us INTEGER,
    updated_at_us INTEGER NOT NULL,
    PRIMARY KEY (username, remote_key)
);

-- +goose Down
DROP TABLE IF EXISTS auth_login_state;
DROP INDEX IF EXISTS browser_sessions_user_active_idx;
DROP TABLE IF EXISTS browser_sessions;
