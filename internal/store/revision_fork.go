package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

// EnsureProjectDraft returns the current Draft revision. If the current
// revision is VALIDATED (normally because it backs the active published
// Snapshot), it creates a complete editable successor revision while leaving
// the validated source and all Runtime Snapshots immutable.
func (s *Store) EnsureProjectDraft(ctx context.Context, projectID, createdBy, changeNote string) (domain.ProjectRevision, error) {
	if err := s.RequireProjectConfigurationMutable(ctx, projectID); err != nil {
		return domain.ProjectRevision{}, err
	}
	project, err := s.GetProject(ctx, projectID)
	if err != nil {
		return domain.ProjectRevision{}, err
	}
	source, err := s.GetRevision(ctx, project.CurrentRevisionID)
	if err != nil {
		return domain.ProjectRevision{}, err
	}
	if source.Status == domain.RevisionDraft {
		return source, nil
	}
	if source.Status != domain.RevisionValidated {
		return domain.ProjectRevision{}, fmt.Errorf("%w: current revision cannot be forked from %s", domain.ErrConflict, source.Status)
	}

	cues, err := s.ListCues(ctx, source.ID)
	if err != nil {
		return domain.ProjectRevision{}, err
	}
	inputs, err := s.ListInputs(ctx, source.ID)
	if err != nil {
		return domain.ProjectRevision{}, err
	}
	outputs, err := s.ListOutputs(ctx, source.ID)
	if err != nil {
		return domain.ProjectRevision{}, err
	}
	routes, err := s.ListRoutes(ctx, source.ID)
	if err != nil {
		return domain.ProjectRevision{}, err
	}
	executionEnvironments, err := s.ListExecutionEnvironmentManifests(ctx, source.ID)
	if err != nil {
		return domain.ProjectRevision{}, err
	}

	newRevisionID, err := stageid.New()
	if err != nil {
		return domain.ProjectRevision{}, err
	}
	now := s.clock.Now().UTC()
	nowUS := clock.UnixMicros(now)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.ProjectRevision{}, fmt.Errorf("begin draft fork: %w", err)
	}
	defer tx.Rollback()

	var currentRevisionID string
	if err := tx.QueryRowContext(ctx, `SELECT current_revision_id FROM projects WHERE project_id = ?`, projectID).Scan(&currentRevisionID); err != nil {
		if err == sql.ErrNoRows {
			return domain.ProjectRevision{}, domain.ErrNotFound
		}
		return domain.ProjectRevision{}, fmt.Errorf("recheck project revision: %w", err)
	}
	if currentRevisionID != source.ID {
		return domain.ProjectRevision{}, domain.ErrConflict
	}
	var nextNumber int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision_number), 0) + 1 FROM project_revisions WHERE project_id = ?`, projectID).Scan(&nextNumber); err != nil {
		return domain.ProjectRevision{}, fmt.Errorf("next revision number: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO project_revisions
		(revision_id, project_id, revision_number, status, parent_revision_id, created_at_us, created_by, change_note)
		VALUES (?, ?, ?, 'DRAFT', ?, ?, ?, ?)
	`, newRevisionID, projectID, nextNumber, source.ID, nowUS, createdBy, changeNote); err != nil {
		return domain.ProjectRevision{}, fmt.Errorf("insert draft successor: %w", err)
	}

	for _, environment := range executionEnvironments {
		newEnvironmentID, err := stageid.New()
		if err != nil {
			return domain.ProjectRevision{}, err
		}
		canonical, err := executionenv.CanonicalBytes(environment.Manifest)
		if err != nil {
			return domain.ProjectRevision{}, fmt.Errorf("clone execution environment canonical manifest: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO execution_environment_manifests (
				environment_manifest_id, revision_id, environment_key, adapter_key, application_key,
				manifest_json, content_sha256, created_by, created_at_us
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			newEnvironmentID, newRevisionID, environment.Manifest.EnvironmentKey,
			environment.Manifest.AdapterKey, environment.Manifest.Application.Key,
			string(canonical), environment.ContentSHA256, createdBy, nowUS,
		); err != nil {
			return domain.ProjectRevision{}, fmt.Errorf("clone execution environment: %w", err)
		}
	}

	cueIDs := make(map[string]string, len(cues))
	for _, cue := range cues {
		newCueID, err := stageid.New()
		if err != nil {
			return domain.ProjectRevision{}, err
		}
		cueIDs[cue.ID] = newCueID
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO cues
			(cue_id, revision_id, display_label, name, order_index, cue_type, criticality, enabled, execution_policy_json, notes_summary)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, newCueID, newRevisionID, cue.DisplayLabel, cue.Name, cue.OrderIndex, cue.CueType,
			cue.Criticality, boolInt(cue.Enabled), string(cue.ExecutionPolicy), cue.NotesSummary); err != nil {
			return domain.ProjectRevision{}, fmt.Errorf("clone cue: %w", err)
		}
		for _, action := range cue.Actions {
			newActionID, err := stageid.New()
			if err != nil {
				return domain.ProjectRevision{}, err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO actions
				(action_id, cue_id, order_index, execution_mode, target_ref, capability_key,
				 parameters_json, timeout_policy_json, error_policy_json, priority_class, enabled)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, newActionID, newCueID, action.OrderIndex, action.ExecutionMode, action.TargetRef,
				action.CapabilityKey, string(action.Parameters), string(action.TimeoutPolicy),
				string(action.ErrorPolicy), action.PriorityClass, boolInt(action.Enabled)); err != nil {
				return domain.ProjectRevision{}, fmt.Errorf("clone action: %w", err)
			}
		}
	}

	inputIDs := make(map[string]string, len(inputs))
	for _, input := range inputs {
		newID, err := stageid.New()
		if err != nil {
			return domain.ProjectRevision{}, err
		}
		inputIDs[input.ID] = newID
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO input_definitions
			(input_id, revision_id, name, source_ref, event_type, value_schema_json, enabled)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, newID, newRevisionID, input.Name, input.SourceRef, input.EventType, string(input.ValueSchema), boolInt(input.Enabled)); err != nil {
			return domain.ProjectRevision{}, fmt.Errorf("clone input: %w", err)
		}
	}

	outputIDs := make(map[string]string, len(outputs))
	for _, output := range outputs {
		newID, err := stageid.New()
		if err != nil {
			return domain.ProjectRevision{}, err
		}
		outputIDs[output.ID] = newID
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO output_definitions
			(output_id, revision_id, name, target_ref, capability_key, value_schema_json, criticality)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, newID, newRevisionID, output.Name, output.TargetRef, output.CapabilityKey,
			string(output.ValueSchema), output.Criticality); err != nil {
			return domain.ProjectRevision{}, fmt.Errorf("clone output: %w", err)
		}
	}

	for _, route := range routes {
		newRouteID, err := stageid.New()
		if err != nil {
			return domain.ProjectRevision{}, err
		}
		newInputID, ok := inputIDs[route.InputID]
		if !ok {
			return domain.ProjectRevision{}, fmt.Errorf("%w: route input missing during draft fork", domain.ErrConflict)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO routes
			(route_id, revision_id, name, input_id, condition_definition_json, transform_definition_json,
			 delay_ms, debounce_ms, priority_class, error_policy_json, enabled)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, newRouteID, newRevisionID, route.Name, newInputID, string(route.ConditionDefinition),
			string(route.TransformDefinition), route.DelayMS, route.DebounceMS, route.PriorityClass,
			string(route.ErrorPolicy), boolInt(route.Enabled)); err != nil {
			return domain.ProjectRevision{}, fmt.Errorf("clone route: %w", err)
		}
		for _, action := range route.Actions {
			newRouteActionID, err := stageid.New()
			if err != nil {
				return domain.ProjectRevision{}, err
			}
			var outputID, cueID any
			if action.OutputID != nil {
				mapped, ok := outputIDs[*action.OutputID]
				if !ok {
					return domain.ProjectRevision{}, fmt.Errorf("%w: route output missing during draft fork", domain.ErrConflict)
				}
				outputID = mapped
			}
			if action.CueID != nil {
				mapped, ok := cueIDs[*action.CueID]
				if !ok {
					return domain.ProjectRevision{}, fmt.Errorf("%w: route cue missing during draft fork", domain.ErrConflict)
				}
				cueID = mapped
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO route_actions
				(route_action_id, route_id, order_index, output_id, cue_id, parameters_json)
				VALUES (?, ?, ?, ?, ?, ?)
			`, newRouteActionID, newRouteID, action.OrderIndex, outputID, cueID, string(action.Parameters)); err != nil {
				return domain.ProjectRevision{}, fmt.Errorf("clone route action: %w", err)
			}
		}
	}

	if _, err := tx.ExecContext(ctx, `UPDATE projects SET current_revision_id = ?, updated_at_us = ? WHERE project_id = ?`, newRevisionID, nowUS, projectID); err != nil {
		return domain.ProjectRevision{}, fmt.Errorf("activate draft successor: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.ProjectRevision{}, fmt.Errorf("commit draft fork: %w", err)
	}
	return domain.ProjectRevision{
		ID: newRevisionID, ProjectID: projectID, RevisionNumber: nextNumber,
		Status: domain.RevisionDraft, ParentRevisionID: &source.ID,
		CreatedAt: now, CreatedBy: createdBy, ChangeNote: changeNote,
	}, nil
}
