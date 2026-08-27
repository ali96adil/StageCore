-- +goose Up
CREATE TABLE operator_notes (
    note_id TEXT PRIMARY KEY CHECK(length(note_id) = 36),
    project_id TEXT NOT NULL,
    session_id TEXT NULL,
    cue_id TEXT NULL,
    category TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL CHECK(length(trim(body)) > 0),
    status TEXT NOT NULL CHECK(status IN ('OPEN', 'RESOLVED')),
    created_by TEXT NOT NULL DEFAULT '',
    created_at_us INTEGER NOT NULL,
    updated_at_us INTEGER NOT NULL,
    resolved_at_us INTEGER NULL,
    FOREIGN KEY (project_id) REFERENCES projects(project_id) ON DELETE RESTRICT,
    FOREIGN KEY (session_id) REFERENCES sessions(session_id) ON DELETE RESTRICT,
    FOREIGN KEY (cue_id) REFERENCES cues(cue_id) ON DELETE RESTRICT
);

CREATE INDEX operator_notes_project_status_idx
    ON operator_notes(project_id, status, updated_at_us DESC);
CREATE INDEX operator_notes_session_idx
    ON operator_notes(session_id, updated_at_us DESC)
    WHERE session_id IS NOT NULL;
CREATE INDEX operator_notes_cue_idx
    ON operator_notes(cue_id, updated_at_us DESC)
    WHERE cue_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS operator_notes_cue_idx;
DROP INDEX IF EXISTS operator_notes_session_idx;
DROP INDEX IF EXISTS operator_notes_project_status_idx;
DROP TABLE IF EXISTS operator_notes;
