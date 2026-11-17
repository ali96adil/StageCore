-- +goose Up
ALTER TABLE sessions ADD COLUMN rehearsal_purpose TEXT NOT NULL DEFAULT ''
    CHECK (rehearsal_purpose = '' OR
           (session_type = 'REHEARSAL' AND rehearsal_purpose IN ('PREVIEW', 'TIMING_CAPTURE')));

-- +goose Down
ALTER TABLE sessions DROP COLUMN rehearsal_purpose;
