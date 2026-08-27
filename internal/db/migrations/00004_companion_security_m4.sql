-- +goose Up
CREATE TABLE companion_pairing_requests (
    pairing_request_id TEXT PRIMARY KEY CHECK(length(pairing_request_id) = 36),
    companion_id TEXT NOT NULL,
    public_key_algorithm TEXT NOT NULL CHECK(public_key_algorithm = 'P256_X963_SHA256'),
    public_key_base64 TEXT NOT NULL,
    public_key_fingerprint TEXT NOT NULL CHECK(length(public_key_fingerprint) = 64),
    client_nonce_hash TEXT NOT NULL CHECK(length(client_nonce_hash) = 64),
    pairing_code_hash TEXT NOT NULL CHECK(length(pairing_code_hash) = 64),
    status TEXT NOT NULL CHECK(status IN ('PENDING', 'APPROVED', 'REJECTED', 'EXPIRED')),
    requested_at_us INTEGER NOT NULL,
    expires_at_us INTEGER NOT NULL,
    approved_at_us INTEGER NULL,
    approved_by TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (companion_id) REFERENCES companions(companion_id) ON DELETE RESTRICT
);

CREATE INDEX companion_pairing_requests_companion_idx
    ON companion_pairing_requests(companion_id, requested_at_us);

CREATE TABLE companion_device_keys (
    companion_id TEXT PRIMARY KEY,
    public_key_algorithm TEXT NOT NULL CHECK(public_key_algorithm = 'P256_X963_SHA256'),
    public_key_base64 TEXT NOT NULL,
    public_key_fingerprint TEXT NOT NULL UNIQUE CHECK(length(public_key_fingerprint) = 64),
    paired_at_us INTEGER NOT NULL,
    revoked_at_us INTEGER NULL,
    FOREIGN KEY (companion_id) REFERENCES companions(companion_id) ON DELETE RESTRICT
);

CREATE TABLE companion_auth_challenges (
    auth_challenge_id TEXT PRIMARY KEY CHECK(length(auth_challenge_id) = 36),
    companion_id TEXT NOT NULL,
    nonce_base64 TEXT NOT NULL,
    created_at_us INTEGER NOT NULL,
    expires_at_us INTEGER NOT NULL,
    used_at_us INTEGER NULL,
    FOREIGN KEY (companion_id) REFERENCES companions(companion_id) ON DELETE RESTRICT
);

CREATE INDEX companion_auth_challenges_companion_idx
    ON companion_auth_challenges(companion_id, created_at_us);

CREATE TABLE companion_runtime_sessions (
    runtime_session_id TEXT PRIMARY KEY CHECK(length(runtime_session_id) = 36),
    companion_id TEXT NOT NULL,
    credential_hash TEXT NOT NULL UNIQUE CHECK(length(credential_hash) = 64),
    created_at_us INTEGER NOT NULL,
    expires_at_us INTEGER NOT NULL,
    revoked_at_us INTEGER NULL,
    FOREIGN KEY (companion_id) REFERENCES companions(companion_id) ON DELETE RESTRICT
);

CREATE INDEX companion_runtime_sessions_companion_idx
    ON companion_runtime_sessions(companion_id, expires_at_us);

CREATE TABLE companion_security_events (
    security_event_id TEXT PRIMARY KEY CHECK(length(security_event_id) = 36),
    companion_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    actor TEXT NOT NULL DEFAULT '',
    result TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    occurred_at_us INTEGER NOT NULL,
    FOREIGN KEY (companion_id) REFERENCES companions(companion_id) ON DELETE RESTRICT
);

CREATE INDEX companion_security_events_companion_idx
    ON companion_security_events(companion_id, occurred_at_us);

-- +goose Down
DROP INDEX IF EXISTS companion_security_events_companion_idx;
DROP TABLE IF EXISTS companion_security_events;
DROP INDEX IF EXISTS companion_runtime_sessions_companion_idx;
DROP TABLE IF EXISTS companion_runtime_sessions;
DROP INDEX IF EXISTS companion_auth_challenges_companion_idx;
DROP TABLE IF EXISTS companion_auth_challenges;
DROP TABLE IF EXISTS companion_device_keys;
DROP INDEX IF EXISTS companion_pairing_requests_companion_idx;
DROP TABLE IF EXISTS companion_pairing_requests;
