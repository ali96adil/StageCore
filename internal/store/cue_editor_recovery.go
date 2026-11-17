package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

type CueEditorRecovery struct {
	Generation string          `json:"generation"`
	Document   json.RawMessage `json:"document,omitempty"`
}

func (s *Store) CueEditorRecovery(ctx context.Context, projectID, userID string) (CueEditorRecovery, error) {
	var recovery CueEditorRecovery
	var document string
	err := s.db.QueryRowContext(ctx, `
		SELECT generation, document FROM cue_editor_recovery WHERE project_id = ? AND user_id = ?
	`, projectID, userID).Scan(&recovery.Generation, &document)
	if errors.Is(err, sql.ErrNoRows) {
		return CueEditorRecovery{}, nil
	}
	if err == nil {
		recovery.Document = json.RawMessage(document)
	}
	return recovery, err
}

// SaveCueEditorRecovery compares an opaque generation to fence stale tabs.
// Editor recovery lives separately from authored cues and published snapshots.
func (s *Store) SaveCueEditorRecovery(ctx context.Context, projectID, userID, expected string, document json.RawMessage) (CueEditorRecovery, error) {
	if len(document) < 2 || len(document) > 16<<10 || document[0] != '{' || !json.Valid(document) {
		return CueEditorRecovery{}, fmt.Errorf("%w: invalid editor recovery document", domain.ErrInvalidInput)
	}
	generation, err := stageid.New()
	if err != nil {
		return CueEditorRecovery{}, err
	}
	var result sql.Result
	if expected == "" {
		result, err = capability.ExecAuthorized(ctx, s.db, `
			INSERT INTO cue_editor_recovery (project_id, user_id, generation, document)
			VALUES (?, ?, ?, ?) ON CONFLICT(project_id, user_id) DO NOTHING
		`, projectID, userID, generation, string(document))
	} else {
		result, err = capability.ExecAuthorized(ctx, s.db, `
			UPDATE cue_editor_recovery SET generation = ?, document = ?
			WHERE project_id = ? AND user_id = ? AND generation = ?
		`, generation, string(document), projectID, userID, expected)
	}
	if err := requireRecoveryGeneration(result, err); err != nil {
		return CueEditorRecovery{}, err
	}
	return CueEditorRecovery{Generation: generation}, nil
}

func (s *Store) DeleteCueEditorRecovery(ctx context.Context, projectID, userID, expected string) error {
	if expected == "" {
		return fmt.Errorf("%w: recovery generation required", domain.ErrInvalidInput)
	}
	result, err := capability.ExecAuthorized(ctx, s.db, `
		DELETE FROM cue_editor_recovery WHERE project_id = ? AND user_id = ? AND generation = ?
	`, projectID, userID, expected)
	return requireRecoveryGeneration(result, err)
}

func requireRecoveryGeneration(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("%w: editor recovery was changed by another tab", domain.ErrConflict)
	}
	return nil
}
