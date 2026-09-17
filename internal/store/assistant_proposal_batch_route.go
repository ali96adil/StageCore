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

func (s *Store) applyAssistantRouteMutationTx(ctx context.Context, tx *sql.Tx, revisionID string, mutation assistantProposalPreparedMutation, refs map[string]assistantProposalRef) error {
	route := mutation.Route
	route.ID = mutation.entityID
	route.RevisionID = revisionID
	route.Name = strings.TrimSpace(route.Name)
	route.InputID = strings.TrimSpace(route.InputID)
	if route.Name == "" || route.InputID == "" {
		return fmt.Errorf("%w: route name and input are required", domain.ErrInvalidInput)
	}
	belongs, err := s.entityBelongsToRevisionTx(ctx, tx, "input_definitions", "input_id", route.InputID, revisionID)
	if err != nil {
		return err
	}
	if !belongs {
		return fmt.Errorf("%w: route input does not belong to revision", domain.ErrInvalidInput)
	}
	if route.PriorityClass == "" {
		route.PriorityClass = domain.PriorityP2
	}
	condition, err := normalizeJSON(route.ConditionDefinition, "null")
	if err != nil {
		return fmt.Errorf("route condition: %w", err)
	}
	transform, err := normalizeJSON(route.TransformDefinition, "null")
	if err != nil {
		return fmt.Errorf("route transform: %w", err)
	}
	errorPolicy, err := normalizeJSON(route.ErrorPolicy, "{}")
	if err != nil {
		return fmt.Errorf("route error policy: %w", err)
	}

	if mutation.TargetID == "" {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO routes (route_id, revision_id, name, input_id, condition_definition_json, transform_definition_json, delay_ms, debounce_ms, priority_class, error_policy_json, enabled)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			route.ID, revisionID, route.Name, route.InputID, condition, transform, route.DelayMS, route.DebounceMS, route.PriorityClass, errorPolicy, boolInt(route.Enabled)); err != nil {
			return fmt.Errorf("insert assistant proposal route: %w", err)
		}
	} else {
		var one int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM routes WHERE route_id = ? AND revision_id = ?`, route.ID, revisionID).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("check assistant proposal route ownership: %w", err)
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE routes SET name = ?, input_id = ?, condition_definition_json = ?, transform_definition_json = ?, delay_ms = ?, debounce_ms = ?, priority_class = ?, error_policy_json = ?, enabled = ?
			WHERE route_id = ? AND revision_id = ?`,
			route.Name, route.InputID, condition, transform, route.DelayMS, route.DebounceMS, route.PriorityClass, errorPolicy, boolInt(route.Enabled), route.ID, revisionID)
		if err != nil {
			return fmt.Errorf("update assistant proposal route: %w", err)
		}
		if err := requireOneRow(result); err != nil {
			if errors.Is(err, domain.ErrConflict) {
				return domain.ErrNotFound
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM route_actions WHERE route_id = ?`, route.ID); err != nil {
			return fmt.Errorf("replace assistant proposal route actions: %w", err)
		}
	}

	for index, actionValue := range mutation.RouteActions {
		action := actionValue.Action
		cueRef := strings.TrimSpace(actionValue.CueRef)
		if cueRef != "" {
			if action.CueID != nil || action.OutputID != nil {
				return fmt.Errorf("%w: route action cue_ref is exclusive with cue_id/output_id", domain.ErrInvalidInput)
			}
			ref, ok := refs[cueRef]
			if !ok || ref.kind != AssistantProposalMutationCue {
				return fmt.Errorf("%w: route action cue_ref %q does not resolve to a cue operation", domain.ErrInvalidInput, cueRef)
			}
			cueID := ref.id
			action.CueID = &cueID
		}
		if (action.OutputID == nil) == (action.CueID == nil) {
			return fmt.Errorf("%w: route action requires exactly one output or cue", domain.ErrInvalidInput)
		}
		if action.OutputID != nil {
			belongs, err := s.entityBelongsToRevisionTx(ctx, tx, "output_definitions", "output_id", strings.TrimSpace(*action.OutputID), revisionID)
			if err != nil {
				return err
			}
			if !belongs {
				return fmt.Errorf("%w: route output does not belong to revision", domain.ErrInvalidInput)
			}
		}
		if action.CueID != nil {
			belongs, err := s.entityBelongsToRevisionTx(ctx, tx, "cues", "cue_id", strings.TrimSpace(*action.CueID), revisionID)
			if err != nil {
				return err
			}
			if !belongs {
				return fmt.Errorf("%w: route cue does not belong to revision", domain.ErrInvalidInput)
			}
		}
		action.ID, err = stageid.New()
		if err != nil {
			return err
		}
		action.RouteID = route.ID
		action.OrderIndex = index
		params, err := normalizeJSON(action.Parameters, "{}")
		if err != nil {
			return fmt.Errorf("route action parameters: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO route_actions (route_action_id, route_id, order_index, output_id, cue_id, parameters_json)
			VALUES (?, ?, ?, ?, ?, ?)`, action.ID, route.ID, action.OrderIndex, action.OutputID, action.CueID, params); err != nil {
			return fmt.Errorf("insert assistant proposal route action: %w", err)
		}
	}
	return nil
}
