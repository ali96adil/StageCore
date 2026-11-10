package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

type CueActionPreset struct {
	ID     string          `json:"preset_id"`
	Name   string          `json:"name"`
	Action json.RawMessage `json:"action"`
}

func (s *Store) ListCueActionPresets(ctx context.Context, projectID, userID string) ([]CueActionPreset, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT preset_id, name, action FROM cue_action_presets
		WHERE project_id = ? AND user_id = ? ORDER BY name, preset_id
	`, projectID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	presets := []CueActionPreset{}
	for rows.Next() {
		var preset CueActionPreset
		var action string
		if err := rows.Scan(&preset.ID, &preset.Name, &action); err != nil {
			return nil, err
		}
		preset.Action = json.RawMessage(action)
		presets = append(presets, preset)
	}
	return presets, rows.Err()
}

func (s *Store) CreateCueActionPreset(ctx context.Context, projectID, userID, name string, action json.RawMessage) (CueActionPreset, error) {
	id, err := stageid.New()
	if err != nil {
		return CueActionPreset{}, err
	}
	result, err := capability.ExecAuthorized(ctx, s.db, `
		INSERT INTO cue_action_presets (preset_id, project_id, user_id, name, action)
		SELECT ?, ?, ?, ?, ?
		WHERE (SELECT COUNT(*) FROM cue_action_presets WHERE project_id = ? AND user_id = ?) < 32
		ON CONFLICT(project_id, user_id, name) DO NOTHING
	`, id, projectID, userID, name, string(action), projectID, userID)
	if err != nil {
		return CueActionPreset{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return CueActionPreset{}, err
	}
	if count == 0 {
		return CueActionPreset{}, fmt.Errorf("%w: preset name exists or limit reached", domain.ErrConflict)
	}
	return CueActionPreset{ID: id, Name: name, Action: action}, nil
}

func (s *Store) DeleteCueActionPreset(ctx context.Context, projectID, userID, id string) error {
	result, err := capability.ExecAuthorized(ctx, s.db, `
		DELETE FROM cue_action_presets WHERE preset_id = ? AND project_id = ? AND user_id = ?
	`, id, projectID, userID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return domain.ErrNotFound
	}
	return nil
}
