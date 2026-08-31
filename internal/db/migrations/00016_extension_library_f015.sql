-- +goose Up
CREATE TABLE extension_packages (
    package_id TEXT PRIMARY KEY REFERENCES software_packages(package_id) ON DELETE RESTRICT,
    extension_id TEXT NOT NULL,
    version TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('PLUGIN', 'ADDON')),
    source TEXT NOT NULL CHECK (source IN ('OFFICIAL', 'LOCAL', 'COMMUNITY')),
    manifest_json TEXT NOT NULL CHECK (json_valid(manifest_json)),
    manifest_sha256 TEXT NOT NULL CHECK (length(manifest_sha256) = 64),
    registered_by TEXT NOT NULL,
    registered_at_us INTEGER NOT NULL
) STRICT;

CREATE INDEX extension_packages_identity_idx
ON extension_packages(extension_id, version, registered_at_us DESC);

-- +goose Down
DROP INDEX IF EXISTS extension_packages_identity_idx;
DROP TABLE IF EXISTS extension_packages;
