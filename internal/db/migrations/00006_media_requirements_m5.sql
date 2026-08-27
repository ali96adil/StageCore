-- +goose Up
CREATE TABLE machine_role_media_requirements (
    media_requirement_id TEXT PRIMARY KEY CHECK(length(media_requirement_id) = 36),
    machine_role_id TEXT NOT NULL,
    content_version_id TEXT NOT NULL,
    required INTEGER NOT NULL DEFAULT 1 CHECK(required IN (0, 1)),
    created_at_us INTEGER NOT NULL,
    FOREIGN KEY (machine_role_id) REFERENCES machine_roles(machine_role_id) ON DELETE RESTRICT,
    FOREIGN KEY (content_version_id) REFERENCES media_content_versions(content_version_id) ON DELETE RESTRICT,
    UNIQUE (machine_role_id, content_version_id)
);

CREATE INDEX machine_role_media_requirements_role_idx
    ON machine_role_media_requirements(machine_role_id, required);

-- +goose Down
DROP INDEX IF EXISTS machine_role_media_requirements_role_idx;
DROP TABLE IF EXISTS machine_role_media_requirements;
