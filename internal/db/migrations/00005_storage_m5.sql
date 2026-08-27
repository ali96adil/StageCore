-- +goose Up
CREATE TABLE vault_objects (
    content_hash TEXT PRIMARY KEY CHECK(length(content_hash) = 64),
    size_bytes INTEGER NOT NULL CHECK(size_bytes >= 0),
    relative_path TEXT NOT NULL UNIQUE,
    created_at_us INTEGER NOT NULL
);

CREATE TABLE media_assets (
    media_asset_id TEXT PRIMARY KEY CHECK(length(media_asset_id) = 36),
    project_id TEXT NOT NULL,
    name TEXT NOT NULL,
    asset_policy TEXT NOT NULL CHECK(asset_policy IN ('REFERENCE_ONLY', 'MANAGED', 'ARCHIVE_REQUIRED')),
    created_at_us INTEGER NOT NULL,
    updated_at_us INTEGER NOT NULL,
    FOREIGN KEY (project_id) REFERENCES projects(project_id) ON DELETE RESTRICT
);

CREATE INDEX media_assets_project_idx
    ON media_assets(project_id, created_at_us);

CREATE TABLE media_content_versions (
    content_version_id TEXT PRIMARY KEY CHECK(length(content_version_id) = 36),
    media_asset_id TEXT NOT NULL,
    content_hash TEXT NOT NULL CHECK(length(content_hash) = 64),
    original_filename TEXT NOT NULL DEFAULT '',
    size_bytes INTEGER NOT NULL CHECK(size_bytes >= 0),
    created_at_us INTEGER NOT NULL,
    FOREIGN KEY (media_asset_id) REFERENCES media_assets(media_asset_id) ON DELETE RESTRICT,
    FOREIGN KEY (content_hash) REFERENCES vault_objects(content_hash) ON DELETE RESTRICT
);

CREATE INDEX media_content_versions_asset_idx
    ON media_content_versions(media_asset_id, created_at_us);
CREATE INDEX media_content_versions_hash_idx
    ON media_content_versions(content_hash);

CREATE TABLE media_locations (
    media_location_id TEXT PRIMARY KEY CHECK(length(media_location_id) = 36),
    content_version_id TEXT NOT NULL,
    location_type TEXT NOT NULL CHECK(location_type IN ('HUB', 'COMPANION', 'REFERENCE')),
    locator TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('AVAILABLE', 'MISSING', 'MISMATCH', 'FAILED')),
    verified_at_us INTEGER NULL,
    FOREIGN KEY (content_version_id) REFERENCES media_content_versions(content_version_id) ON DELETE RESTRICT
);

CREATE INDEX media_locations_version_idx
    ON media_locations(content_version_id, location_type);

-- +goose Down
DROP INDEX IF EXISTS media_locations_version_idx;
DROP TABLE IF EXISTS media_locations;
DROP INDEX IF EXISTS media_content_versions_hash_idx;
DROP INDEX IF EXISTS media_content_versions_asset_idx;
DROP TABLE IF EXISTS media_content_versions;
DROP INDEX IF EXISTS media_assets_project_idx;
DROP TABLE IF EXISTS media_assets;
DROP TABLE IF EXISTS vault_objects;
