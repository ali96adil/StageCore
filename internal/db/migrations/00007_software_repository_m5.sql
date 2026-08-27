-- +goose Up
CREATE TABLE software_packages (
    package_id TEXT PRIMARY KEY CHECK(length(package_id) = 36),
    product_id TEXT NOT NULL,
    version TEXT NOT NULL,
    platform TEXT NOT NULL,
    architecture TEXT NOT NULL,
    min_api_version INTEGER NOT NULL,
    max_api_version INTEGER NOT NULL,
    content_hash TEXT NOT NULL,
    size_bytes INTEGER NOT NULL CHECK(size_bytes >= 0),
    original_filename TEXT NOT NULL DEFAULT '',
    signing_status TEXT NOT NULL CHECK(signing_status IN ('UNKNOWN', 'UNSIGNED', 'SIGNED')),
    notarization_status TEXT NOT NULL CHECK(notarization_status IN ('UNKNOWN', 'NOT_APPLICABLE', 'NOT_NOTARIZED', 'NOTARIZED')),
    release_channel TEXT NOT NULL CHECK(release_channel IN ('development', 'release')),
    release_notes TEXT NOT NULL DEFAULT '',
    created_at_us INTEGER NOT NULL,
    FOREIGN KEY (content_hash) REFERENCES vault_objects(content_hash) ON DELETE RESTRICT,
    UNIQUE (product_id, version, platform, architecture, release_channel)
);

CREATE INDEX software_packages_compatibility_idx
    ON software_packages(product_id, platform, architecture, min_api_version, max_api_version, release_channel);

-- +goose Down
DROP INDEX IF EXISTS software_packages_compatibility_idx;
DROP TABLE IF EXISTS software_packages;
