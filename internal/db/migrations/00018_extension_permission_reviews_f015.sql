-- +goose Up
CREATE TABLE extension_permission_reviews (
    installation_id TEXT NOT NULL REFERENCES extension_installations(installation_id) ON DELETE RESTRICT,
    permission TEXT NOT NULL CHECK(length(permission) > 0),
    decision TEXT NOT NULL CHECK(decision IN ('APPROVED', 'DENIED')),
    reviewed_by TEXT NOT NULL,
    reviewed_at_us INTEGER NOT NULL,
    PRIMARY KEY (installation_id, permission)
);

CREATE INDEX idx_extension_permission_reviews_reviewed_at
    ON extension_permission_reviews(reviewed_at_us DESC, installation_id, permission);

-- +goose Down
DROP INDEX IF EXISTS idx_extension_permission_reviews_reviewed_at;
DROP TABLE IF EXISTS extension_permission_reviews;
