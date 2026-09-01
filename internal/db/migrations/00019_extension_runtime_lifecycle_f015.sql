-- +goose Up
CREATE TABLE extension_runtime_lifecycle (
    installation_id TEXT PRIMARY KEY REFERENCES extension_installations(installation_id) ON DELETE RESTRICT,
    desired_state TEXT NOT NULL CHECK(desired_state IN ('DISABLED', 'ENABLED')),
    observed_state TEXT NOT NULL CHECK(observed_state IN ('STOPPED', 'STARTING', 'READY', 'FAILED')),
    generation INTEGER NOT NULL CHECK(generation >= 0),
    last_error_code TEXT,
    last_error_message TEXT,
    updated_by TEXT NOT NULL CHECK(length(updated_by) > 0),
    updated_at_us INTEGER NOT NULL,
    observed_at_us INTEGER NOT NULL
);

INSERT INTO extension_runtime_lifecycle (
    installation_id, desired_state, observed_state, generation,
    last_error_code, last_error_message, updated_by, updated_at_us, observed_at_us
)
SELECT
    installation_id, 'DISABLED', 'STOPPED', 0,
    NULL, NULL, installed_by, installed_at_us, installed_at_us
FROM extension_installations;

CREATE INDEX idx_extension_runtime_lifecycle_desired
    ON extension_runtime_lifecycle(desired_state, updated_at_us DESC, installation_id);

-- +goose Down
DROP INDEX IF EXISTS idx_extension_runtime_lifecycle_desired;
DROP TABLE IF EXISTS extension_runtime_lifecycle;
