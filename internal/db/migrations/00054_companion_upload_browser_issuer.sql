-- +goose Up
ALTER TABLE companion_upload_tickets ADD COLUMN issuer_browser_session_id TEXT;
ALTER TABLE companion_upload_tickets ADD COLUMN issuer_requires_project_grants INTEGER NOT NULL DEFAULT 0
    CHECK (issuer_requires_project_grants IN (0, 1) AND
           (issuer_requires_project_grants = 0 OR issuer_browser_session_id IS NOT NULL));

-- +goose Down
UPDATE companion_upload_tickets
SET status = 'CANCELLED', terminal_at_us = MAX(created_at_us, unixepoch() * 1000000)
WHERE status = 'ACTIVE' AND issuer_browser_session_id IS NOT NULL;
ALTER TABLE companion_upload_tickets DROP COLUMN issuer_requires_project_grants;
ALTER TABLE companion_upload_tickets DROP COLUMN issuer_browser_session_id;
