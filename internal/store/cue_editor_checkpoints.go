package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

const maxCueEditorCheckpoints = 20
const maxCueEditorCheckpointNameRunes = 64
const maxCueEditorCheckpointBytes = 16 << 10

type CueEditorCheckpoint struct {
	ID        string          `json:"checkpoint_id"`
	CueID     string          `json:"cue_id"`
	Name      string          `json:"name"`
	CreatedAt string          `json:"created_at"`
	Document  json.RawMessage `json:"document"`
}

func validCueEditorCheckpointDocument(cueID string, document json.RawMessage) bool {
	trimmed := bytes.TrimSpace(document)
	if len(trimmed) < 2 || len(trimmed) > maxCueEditorCheckpointBytes || trimmed[0] != '{' || !json.Valid(trimmed) {
		return false
	}
	var envelope struct {
		Version int             `json:"version"`
		CueID   string          `json:"cue_id"`
		Draft   json.RawMessage `json:"draft"`
	}
	if err := json.Unmarshal(trimmed, &envelope); err != nil || envelope.Version != 1 || envelope.CueID != cueID {
		return false
	}
	draft := bytes.TrimSpace(envelope.Draft)
	if len(draft) < 2 || draft[0] != '{' || !json.Valid(draft) {
		return false
	}
	return true
}

func (s *Store) CreateCueEditorCheckpoint(ctx context.Context, projectID, userID, cueID, name string, document json.RawMessage) (CueEditorCheckpoint, error) {
	name = strings.TrimSpace(name)
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(userID) == "" || strings.TrimSpace(cueID) == "" ||
		name == "" || utf8.RuneCountInString(name) > maxCueEditorCheckpointNameRunes ||
		strings.ContainsFunc(name, unicode.IsControl) || !validCueEditorCheckpointDocument(cueID, document) {
		return CueEditorCheckpoint{}, fmt.Errorf("%w: invalid Cue editor checkpoint", domain.ErrInvalidInput)
	}
	var one int
	err := s.db.QueryRowContext(ctx, `
		SELECT 1 FROM cues c
		JOIN project_revisions r ON r.revision_id = c.revision_id
		WHERE c.cue_id = ? AND r.project_id = ?
	`, cueID, projectID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return CueEditorCheckpoint{}, domain.ErrNotFound
	}
	if err != nil {
		return CueEditorCheckpoint{}, fmt.Errorf("verify Cue editor checkpoint scope: %w", err)
	}
	id, err := stageid.New()
	if err != nil {
		return CueEditorCheckpoint{}, err
	}
	createdAt := s.clock.Now().UTC().Format("2006-01-02T15:04:05.000000000Z07:00")
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CueEditorCheckpoint{}, fmt.Errorf("begin Cue editor checkpoint: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO cue_editor_checkpoints (checkpoint_id, project_id, user_id, cue_id, name, created_at, document)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, projectID, userID, cueID, name, createdAt, string(document)); err != nil {
		return CueEditorCheckpoint{}, fmt.Errorf("insert Cue editor checkpoint: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM cue_editor_checkpoints
		WHERE project_id = ? AND user_id = ? AND cue_id = ? AND checkpoint_order NOT IN (
			SELECT checkpoint_order FROM cue_editor_checkpoints
			WHERE project_id = ? AND user_id = ? AND cue_id = ?
			ORDER BY checkpoint_order DESC LIMIT ?
		)
	`, projectID, userID, cueID, projectID, userID, cueID, maxCueEditorCheckpoints); err != nil {
		return CueEditorCheckpoint{}, fmt.Errorf("retain bounded Cue editor checkpoints: %w", err)
	}
	if err := capability.CheckWriteAuthorization(ctx, tx); err != nil {
		return CueEditorCheckpoint{}, err
	}
	if err := tx.Commit(); err != nil {
		return CueEditorCheckpoint{}, fmt.Errorf("commit Cue editor checkpoint: %w", err)
	}
	return CueEditorCheckpoint{ID: id, CueID: cueID, Name: name, CreatedAt: createdAt, Document: append(json.RawMessage(nil), document...)}, nil
}

func (s *Store) ListCueEditorCheckpoints(ctx context.Context, projectID, userID, cueID string) ([]CueEditorCheckpoint, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT checkpoint_id, cue_id, name, created_at, document
		FROM cue_editor_checkpoints
		WHERE project_id = ? AND user_id = ? AND cue_id = ?
		ORDER BY checkpoint_order DESC
	`, projectID, userID, cueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	checkpoints := []CueEditorCheckpoint{}
	for rows.Next() {
		var checkpoint CueEditorCheckpoint
		var document string
		if err := rows.Scan(&checkpoint.ID, &checkpoint.CueID, &checkpoint.Name, &checkpoint.CreatedAt, &document); err != nil {
			return nil, err
		}
		checkpoint.Document = json.RawMessage(document)
		checkpoints = append(checkpoints, checkpoint)
	}
	return checkpoints, rows.Err()
}

func (s *Store) DeleteCueEditorCheckpoint(ctx context.Context, projectID, userID, cueID, checkpointID string) error {
	result, err := capability.ExecAuthorized(ctx, s.db, `
		DELETE FROM cue_editor_checkpoints
		WHERE checkpoint_id = ? AND project_id = ? AND user_id = ? AND cue_id = ?
	`, checkpointID, projectID, userID, cueID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return domain.ErrNotFound
	}
	return nil
}
