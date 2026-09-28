-- +goose Up
CREATE TABLE companion_upload_tickets (
    upload_ticket_id TEXT PRIMARY KEY CHECK(length(upload_ticket_id) = 36),
    credential_hash TEXT NOT NULL UNIQUE CHECK(length(credential_hash) = 64),
    companion_id TEXT NOT NULL,
    runtime_session_id TEXT NOT NULL,
    operation_id TEXT NOT NULL CHECK(length(operation_id) BETWEEN 1 AND 128),
    environment_manifest_id TEXT NOT NULL,
    machine_role_id TEXT NOT NULL,
    runtime_snapshot_id TEXT NOT NULL,
    purpose TEXT NOT NULL CHECK(purpose IN ('EXECUTION_ENVIRONMENT_CAPTURE')),
    expected_content_hash TEXT NOT NULL CHECK(length(expected_content_hash) = 64),
    expected_size_bytes INTEGER NOT NULL CHECK(expected_size_bytes >= 0),
    status TEXT NOT NULL CHECK(status IN ('ACTIVE', 'COMPLETED', 'CANCELLED', 'EXPIRED')),
    created_at_us INTEGER NOT NULL,
    expires_at_us INTEGER NOT NULL CHECK(expires_at_us > created_at_us),
    terminal_at_us INTEGER NULL,
    CHECK (
        (status = 'ACTIVE' AND terminal_at_us IS NULL)
        OR (status <> 'ACTIVE' AND terminal_at_us IS NOT NULL)
    ),
    FOREIGN KEY (companion_id) REFERENCES companions(companion_id) ON DELETE RESTRICT,
    FOREIGN KEY (runtime_session_id) REFERENCES companion_runtime_sessions(runtime_session_id) ON DELETE RESTRICT,
    FOREIGN KEY (environment_manifest_id) REFERENCES execution_environment_manifests(environment_manifest_id) ON DELETE RESTRICT,
    FOREIGN KEY (machine_role_id) REFERENCES machine_roles(machine_role_id) ON DELETE RESTRICT,
    FOREIGN KEY (runtime_snapshot_id) REFERENCES runtime_snapshots(runtime_snapshot_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX companion_upload_tickets_active_operation_idx
    ON companion_upload_tickets(companion_id, operation_id, purpose)
    WHERE status = 'ACTIVE';

CREATE INDEX companion_upload_tickets_companion_status_idx
    ON companion_upload_tickets(companion_id, status, expires_at_us);

-- Ticket creation is authority-sensitive. The environment, role and required
-- Runtime Snapshot must belong to one Project, and the trusted Companion must
-- currently own that role. Later terminal status changes intentionally do not
-- re-run this trigger so authority loss can still cancel/expire an existing row.
-- +goose StatementBegin
CREATE TRIGGER companion_upload_ticket_scope_insert
BEFORE INSERT ON companion_upload_tickets
WHEN NOT EXISTS (
    SELECT 1
    FROM execution_environment_manifests eem
    JOIN project_revisions pr
      ON pr.revision_id = eem.revision_id
    JOIN machine_roles mr
      ON mr.machine_role_id = NEW.machine_role_id
     AND mr.project_id = pr.project_id
    JOIN runtime_snapshots rs
      ON rs.runtime_snapshot_id = NEW.runtime_snapshot_id
     AND rs.project_id = pr.project_id
    JOIN companions c
      ON c.companion_id = NEW.companion_id
     AND c.trust_state = 'TRUSTED'
    JOIN role_assignments ra
      ON ra.machine_role_id = mr.machine_role_id
     AND ra.companion_id = c.companion_id
     AND ra.state <> 'RELEASED'
    JOIN companion_runtime_sessions crs
      ON crs.runtime_session_id = NEW.runtime_session_id
     AND crs.companion_id = c.companion_id
     AND crs.revoked_at_us IS NULL
     AND crs.expires_at_us > NEW.created_at_us
    WHERE eem.environment_manifest_id = NEW.environment_manifest_id
      AND mr.required_runtime_snapshot_id = rs.runtime_snapshot_id
)
BEGIN
    SELECT RAISE(ABORT, 'COMPANION_UPLOAD_TICKET_SCOPE_MISMATCH');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER companion_upload_ticket_scope_update
BEFORE UPDATE OF companion_id, runtime_session_id, environment_manifest_id, machine_role_id, runtime_snapshot_id
ON companion_upload_tickets
WHEN NOT EXISTS (
    SELECT 1
    FROM execution_environment_manifests eem
    JOIN project_revisions pr
      ON pr.revision_id = eem.revision_id
    JOIN machine_roles mr
      ON mr.machine_role_id = NEW.machine_role_id
     AND mr.project_id = pr.project_id
    JOIN runtime_snapshots rs
      ON rs.runtime_snapshot_id = NEW.runtime_snapshot_id
     AND rs.project_id = pr.project_id
    JOIN companions c
      ON c.companion_id = NEW.companion_id
     AND c.trust_state = 'TRUSTED'
    JOIN role_assignments ra
      ON ra.machine_role_id = mr.machine_role_id
     AND ra.companion_id = c.companion_id
     AND ra.state <> 'RELEASED'
    JOIN companion_runtime_sessions crs
      ON crs.runtime_session_id = NEW.runtime_session_id
     AND crs.companion_id = c.companion_id
     AND crs.revoked_at_us IS NULL
     AND crs.expires_at_us > NEW.created_at_us
    WHERE eem.environment_manifest_id = NEW.environment_manifest_id
      AND mr.required_runtime_snapshot_id = rs.runtime_snapshot_id
)
BEGIN
    SELECT RAISE(ABORT, 'COMPANION_UPLOAD_TICKET_SCOPE_MISMATCH');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS companion_upload_ticket_scope_update;
DROP TRIGGER IF EXISTS companion_upload_ticket_scope_insert;
DROP INDEX IF EXISTS companion_upload_tickets_companion_status_idx;
DROP INDEX IF EXISTS companion_upload_tickets_active_operation_idx;
DROP TABLE IF EXISTS companion_upload_tickets;
