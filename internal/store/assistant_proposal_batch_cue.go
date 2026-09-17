package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

func (s *Store) applyAssistantCueMutationTx(ctx context.Context, tx *sql.Tx, revisionID string, mutation assistantProposalPreparedMutation) error {
	cue := mutation.Cue
	cue.ID = mutation.entityID
	cue.RevisionID = revisionID
	cue.Name = strings.TrimSpace(cue.Name)
	if cue.Name == "" {
		return fmt.Errorf("%w: cue name is required", domain.ErrInvalidInput)
	}
	if cue.CueType == "" {
		cue.CueType = "STANDARD"
	}
	if cue.Criticality == "" {
		cue.Criticality = "NORMAL"
	}
	policy, err := normalizeJSON(cue.ExecutionPolicy, "{}")
	if err != nil {
		return fmt.Errorf("cue execution policy: %w", err)
	}

	existingActionIDs := make(map[string]struct{})
	if mutation.TargetID == "" {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO cues (cue_id, revision_id, display_label, name, order_index, cue_type, criticality, enabled, execution_policy_json, notes_summary)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			cue.ID, revisionID, cue.DisplayLabel, cue.Name, cue.OrderIndex, cue.CueType, cue.Criticality, boolInt(cue.Enabled), policy, cue.NotesSummary); err != nil {
			return fmt.Errorf("insert assistant proposal cue: %w", err)
		}
	} else {
		belongs, err := s.entityBelongsToRevisionTx(ctx, tx, "cues", "cue_id", cue.ID, revisionID)
		if err != nil {
			return err
		}
		if !belongs {
			return domain.ErrNotFound
		}
		rows, err := tx.QueryContext(ctx, `SELECT action_id FROM actions WHERE cue_id = ?`, cue.ID)
		if err != nil {
			return fmt.Errorf("list assistant proposal cue action ids: %w", err)
		}
		for rows.Next() {
			var actionID string
			if err := rows.Scan(&actionID); err != nil {
				_ = rows.Close()
				return err
			}
			existingActionIDs[actionID] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		_ = rows.Close()
		result, err := tx.ExecContext(ctx, `
			UPDATE cues SET display_label = ?, name = ?, order_index = ?, cue_type = ?, criticality = ?, enabled = ?, execution_policy_json = ?, notes_summary = ?
			WHERE cue_id = ? AND revision_id = ?`,
			cue.DisplayLabel, cue.Name, cue.OrderIndex, cue.CueType, cue.Criticality, boolInt(cue.Enabled), policy, cue.NotesSummary, cue.ID, revisionID)
		if err != nil {
			return fmt.Errorf("update assistant proposal cue: %w", err)
		}
		if err := requireOneRow(result); err != nil {
			if errors.Is(err, domain.ErrConflict) {
				return domain.ErrNotFound
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM actions WHERE cue_id = ?`, cue.ID); err != nil {
			return fmt.Errorf("replace assistant proposal cue actions: %w", err)
		}
	}

	for _, actionValue := range mutation.CueActions {
		action := actionValue
		action.CueID = cue.ID
		if action.ID != "" && mutation.TargetID != "" {
			if _, ok := existingActionIDs[action.ID]; !ok {
				action.ID = ""
			}
		}
		if action.ID == "" {
			action.ID, err = stageid.New()
			if err != nil {
				return err
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
			return fmt.Errorf("action parameters: %w", err)
		}
		timeoutPolicy, err := normalizeJSON(action.TimeoutPolicy, "{}")
		if err != nil {
			return fmt.Errorf("action timeout policy: %w", err)
		}
		errorPolicy, err := normalizeJSON(action.ErrorPolicy, "{}")
		if err != nil {
			return fmt.Errorf("action error policy: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO actions (action_id, cue_id, order_index, execution_mode, target_ref, capability_key, parameters_json, timeout_policy_json, error_policy_json, priority_class, enabled)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			action.ID, cue.ID, action.OrderIndex, action.ExecutionMode, strings.TrimSpace(action.TargetRef), strings.TrimSpace(action.CapabilityKey),
			params, timeoutPolicy, errorPolicy, action.PriorityClass, boolInt(action.Enabled)); err != nil {
			return fmt.Errorf("insert assistant proposal action: %w", err)
		}
	}
	return nil
}
