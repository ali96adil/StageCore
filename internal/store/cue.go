package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

func (s *Store) CreateCueWithActions(ctx context.Context, cue domain.Cue, actions []domain.Action) (domain.Cue, error) {
	if strings.TrimSpace(cue.RevisionID) == "" || strings.TrimSpace(cue.Name) == "" {
		return domain.Cue{}, fmt.Errorf("%w: cue revision and name are required", domain.ErrInvalidInput)
	}
	if err := s.ensureDraft(ctx, s.db, cue.RevisionID); err != nil {
		return domain.Cue{}, err
	}
	if cue.ID == "" {
		var err error
		cue.ID, err = stageid.New()
		if err != nil {
			return domain.Cue{}, err
		}
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
		return domain.Cue{}, fmt.Errorf("begin create cue: %w", err)
	}
	defer tx.Rollback()
	if err := s.ensureDraft(ctx, tx, cue.RevisionID); err != nil {
		return domain.Cue{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO cues (cue_id, revision_id, display_label, name, order_index, cue_type, criticality, enabled, execution_policy_json, notes_summary)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cue.ID, cue.RevisionID, cue.DisplayLabel, strings.TrimSpace(cue.Name), cue.OrderIndex, cue.CueType, cue.Criticality, boolInt(cue.Enabled), policy, cue.NotesSummary); err != nil {
		return domain.Cue{}, fmt.Errorf("insert cue: %w", err)
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
			INSERT INTO actions (action_id, cue_id, order_index, execution_mode, target_ref, capability_key, parameters_json, timeout_policy_json, error_policy_json, priority_class, enabled)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			action.ID, action.CueID, action.OrderIndex, action.ExecutionMode, action.TargetRef, action.CapabilityKey, params, timeoutPolicy, errorPolicy, action.PriorityClass, boolInt(action.Enabled)); err != nil {
			return domain.Cue{}, fmt.Errorf("insert action: %w", err)
		}
		action.Parameters = json.RawMessage(params)
		action.TimeoutPolicy = json.RawMessage(timeoutPolicy)
		action.ErrorPolicy = json.RawMessage(errorPolicy)
		cue.Actions = append(cue.Actions, action)
	}
	if err := tx.Commit(); err != nil {
		return domain.Cue{}, fmt.Errorf("commit create cue: %w", err)
	}
	cue.ExecutionPolicy = json.RawMessage(policy)
	return cue, nil
}

func (s *Store) ListCues(ctx context.Context, revisionID string) ([]domain.Cue, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT cue_id, revision_id, display_label, name, order_index, cue_type, criticality, enabled, execution_policy_json, notes_summary
		FROM cues WHERE revision_id = ? ORDER BY order_index, cue_id`, revisionID)
	if err != nil {
		return nil, fmt.Errorf("list cues: %w", err)
	}
	var cues []domain.Cue
	for rows.Next() {
		var cue domain.Cue
		var enabled int
		var policy string
		if err := rows.Scan(&cue.ID, &cue.RevisionID, &cue.DisplayLabel, &cue.Name, &cue.OrderIndex, &cue.CueType, &cue.Criticality, &enabled, &policy, &cue.NotesSummary); err != nil {
			return nil, fmt.Errorf("scan cue: %w", err)
		}
		cue.Enabled = enabled == 1
		cue.ExecutionPolicy = json.RawMessage(policy)
		cues = append(cues, cue)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate cues: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close cue rows: %w", err)
	}
	for i := range cues {
		actions, err := s.listActions(ctx, cues[i].ID)
		if err != nil {
			return nil, err
		}
		cues[i].Actions = actions
	}
	return cues, nil
}

func (s *Store) listActions(ctx context.Context, cueID string) ([]domain.Action, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT action_id, cue_id, order_index, execution_mode, target_ref, capability_key, parameters_json, timeout_policy_json, error_policy_json, priority_class, enabled
		FROM actions WHERE cue_id = ? ORDER BY order_index, action_id`, cueID)
	if err != nil {
		return nil, fmt.Errorf("list actions: %w", err)
	}
	defer rows.Close()
	var actions []domain.Action
	for rows.Next() {
		var a domain.Action
		var params, timeoutPolicy, errorPolicy, priority string
		var enabled int
		if err := rows.Scan(&a.ID, &a.CueID, &a.OrderIndex, &a.ExecutionMode, &a.TargetRef, &a.CapabilityKey, &params, &timeoutPolicy, &errorPolicy, &priority, &enabled); err != nil {
			return nil, fmt.Errorf("scan action: %w", err)
		}
		a.Parameters = json.RawMessage(params)
		a.TimeoutPolicy = json.RawMessage(timeoutPolicy)
		a.ErrorPolicy = json.RawMessage(errorPolicy)
		a.PriorityClass = domain.PriorityClass(priority)
		a.Enabled = enabled == 1
		actions = append(actions, a)
	}
	return actions, rows.Err()
}
