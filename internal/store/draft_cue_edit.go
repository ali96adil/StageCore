package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

// GetCue returns one Cue and its Actions from a revision-backed Draft or
// published definition. Mutation callers must still enforce ensureDraft.
func (s *Store) GetCue(ctx context.Context, cueID string) (domain.Cue, error) {
	var cue domain.Cue
	var enabled int
	var policy string
	err := s.db.QueryRowContext(ctx, `
		SELECT cue_id, revision_id, display_label, name, order_index, cue_type,
		       criticality, enabled, execution_policy_json, notes_summary
		FROM cues WHERE cue_id = ?
	`, cueID).Scan(
		&cue.ID, &cue.RevisionID, &cue.DisplayLabel, &cue.Name, &cue.OrderIndex,
		&cue.CueType, &cue.Criticality, &enabled, &policy, &cue.NotesSummary,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Cue{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Cue{}, fmt.Errorf("get cue: %w", err)
	}
	cue.Enabled = enabled == 1
	cue.ExecutionPolicy = json.RawMessage(policy)
	actions, err := s.listActions(ctx, cue.ID)
	if err != nil {
		return domain.Cue{}, err
	}
	cue.Actions = actions
	return cue, nil
}

// ReplaceDraftCue replaces the editable Cue fields and its Action list as one
// transaction. IDs supplied for Actions are preserved; missing IDs receive a
// fresh UUIDv7. Published/frozen revisions are never modified.
func (s *Store) ReplaceDraftCue(ctx context.Context, cue domain.Cue, actions []domain.Action) (domain.Cue, error) {
	if strings.TrimSpace(cue.ID) == "" || strings.TrimSpace(cue.RevisionID) == "" || strings.TrimSpace(cue.Name) == "" {
		return domain.Cue{}, fmt.Errorf("%w: cue ID, revision and name are required", domain.ErrInvalidInput)
	}
	if cue.CueType == "" {
		cue.CueType = "STANDARD"
	}
	if cue.Criticality == "" {
		cue.Criticality = "NORMAL"
	}
	policy, err := normalizeJSON(cue.ExecutionPolicy, "{}")
	if err != nil {
		return domain.Cue{}, fmt.Errorf("cue execution policy: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Cue{}, fmt.Errorf("begin replace cue: %w", err)
	}
	defer tx.Rollback()
	if err := s.ensureDraft(ctx, tx, cue.RevisionID); err != nil {
		return domain.Cue{}, err
	}
	belongs, err := s.entityBelongsToRevisionTx(ctx, tx, "cues", "cue_id", cue.ID, cue.RevisionID)
	if err != nil {
		return domain.Cue{}, err
	}
	if !belongs {
		return domain.Cue{}, domain.ErrNotFound
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE cues SET display_label = ?, name = ?, order_index = ?, cue_type = ?,
		criticality = ?, enabled = ?, execution_policy_json = ?, notes_summary = ?
		WHERE cue_id = ? AND revision_id = ?
	`, cue.DisplayLabel, strings.TrimSpace(cue.Name), cue.OrderIndex, cue.CueType,
		cue.Criticality, boolInt(cue.Enabled), policy, cue.NotesSummary, cue.ID, cue.RevisionID)
	if err != nil {
		return domain.Cue{}, fmt.Errorf("update cue: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return domain.Cue{}, fmt.Errorf("cue rows affected: %w", err)
		}
		return domain.Cue{}, domain.ErrConflict
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM actions WHERE cue_id = ?`, cue.ID); err != nil {
		return domain.Cue{}, fmt.Errorf("replace cue actions: %w", err)
	}

	cue.Actions = make([]domain.Action, 0, len(actions))
	for _, action := range actions {
		action.CueID = cue.ID
		if action.ID == "" {
			action.ID, err = stageid.New()
			if err != nil {
				return domain.Cue{}, err
			}
		}
		if action.ExecutionMode == "" {
			action.ExecutionMode = "SEQUENTIAL"
		}
		if action.PriorityClass == "" {
			action.PriorityClass = domain.PriorityP1
		}
		params, err := normalizeJSON(action.Parameters, "{}")
		if err != nil {
			return domain.Cue{}, fmt.Errorf("action parameters: %w", err)
		}
		timeoutPolicy, err := normalizeJSON(action.TimeoutPolicy, "{}")
		if err != nil {
			return domain.Cue{}, fmt.Errorf("action timeout policy: %w", err)
		}
		errorPolicy, err := normalizeJSON(action.ErrorPolicy, "{}")
		if err != nil {
			return domain.Cue{}, fmt.Errorf("action error policy: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO actions (action_id, cue_id, order_index, execution_mode, target_ref,
			 capability_key, parameters_json, timeout_policy_json, error_policy_json,
			 priority_class, enabled)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, action.ID, cue.ID, action.OrderIndex, action.ExecutionMode, strings.TrimSpace(action.TargetRef),
			strings.TrimSpace(action.CapabilityKey), params, timeoutPolicy, errorPolicy,
			action.PriorityClass, boolInt(action.Enabled)); err != nil {
			return domain.Cue{}, fmt.Errorf("insert replacement action: %w", err)
		}
		action.Parameters = json.RawMessage(params)
		action.TimeoutPolicy = json.RawMessage(timeoutPolicy)
		action.ErrorPolicy = json.RawMessage(errorPolicy)
		cue.Actions = append(cue.Actions, action)
	}
	if err := tx.Commit(); err != nil {
		return domain.Cue{}, fmt.Errorf("commit replace cue: %w", err)
	}
	cue.ExecutionPolicy = json.RawMessage(policy)
	return cue, nil
}

func (s *Store) DeleteDraftCue(ctx context.Context, revisionID, cueID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete cue: %w", err)
	}
	defer tx.Rollback()
	if err := s.ensureDraft(ctx, tx, revisionID); err != nil {
		return err
	}
	belongs, err := s.entityBelongsToRevisionTx(ctx, tx, "cues", "cue_id", cueID, revisionID)
	if err != nil {
		return err
	}
	if !belongs {
		return domain.ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM cues WHERE cue_id = ? AND revision_id = ?`, cueID, revisionID); err != nil {
		return fmt.Errorf("delete cue: %w", err)
	}
	return tx.Commit()
}

func (s *Store) DuplicateDraftCue(ctx context.Context, revisionID, cueID, displayLabel, name string, orderIndex int) (domain.Cue, error) {
	if err := s.ensureDraft(ctx, s.db, revisionID); err != nil {
		return domain.Cue{}, err
	}
	source, err := s.GetCue(ctx, cueID)
	if err != nil {
		return domain.Cue{}, err
	}
	if source.RevisionID != revisionID {
		return domain.Cue{}, domain.ErrNotFound
	}
	copyCue := source
	copyCue.ID = ""
	copyCue.DisplayLabel = displayLabel
	copyCue.Name = strings.TrimSpace(name)
	copyCue.OrderIndex = orderIndex
	if copyCue.Name == "" {
		copyCue.Name = source.Name + " Copy"
	}
	actions := make([]domain.Action, len(source.Actions))
	for i, sourceAction := range source.Actions {
		actions[i] = sourceAction
		actions[i].ID = ""
		actions[i].CueID = ""
	}
	return s.CreateCueWithActions(ctx, copyCue, actions)
}
