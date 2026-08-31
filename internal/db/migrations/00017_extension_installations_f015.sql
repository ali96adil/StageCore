-- +goose Up
CREATE TABLE extension_installations (
    installation_id TEXT PRIMARY KEY,
    package_id TEXT NOT NULL UNIQUE REFERENCES extension_packages(package_id) ON DELETE RESTRICT,
    lifecycle_state TEXT NOT NULL CHECK(lifecycle_state = 'INSTALLED'),
    payload_relative_path TEXT NOT NULL,
    content_sha256 TEXT NOT NULL CHECK(length(content_sha256) = 64),
    size_bytes INTEGER NOT NULL CHECK(size_bytes >= 0),
    installed_by TEXT NOT NULL,
    installed_at_us INTEGER NOT NULL
);

CREATE INDEX idx_extension_installations_installed_at
    ON extension_installations(installed_at_us DESC, installation_id DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_extension_installations_installed_at;
DROP TABLE IF EXISTS extension_installations;
