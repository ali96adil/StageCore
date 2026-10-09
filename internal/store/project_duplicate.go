package store
import (
 "context"
 "database/sql"
 "fmt"
 "strings"
 "github.com/ali96adil/StageCore/internal/clock"
 "github.com/ali96adil/StageCore/internal/domain"
 "github.com/ali96adil/StageCore/internal/executionenv"
 stageid "github.com/ali96adil/StageCore/internal/id"
)
// DuplicateProjectAuthoring is a backend-only core graph clone. Not full duplication yet.
func (s *Store) DuplicateProjectAuthoring(ctx context.Context, sourceProjectID, newName, createdBy string) (domain.Project, domain.ProjectRevision, error) {
	newName = strings.TrimSpace(newName)
    if newName == "" || len(newName) > 160 || strings.TrimSpace(createdBy) == "" { return domain.Project{}, domain.ProjectRevision{}, domain.ErrInvalidInput }
    project, err := s.GetProject(ctx, sourceProjectID)
	if err != nil {
		return domain.Project{}, domain.ProjectRevision{}, err
	}
	source, err := s.GetRevision(ctx, project.CurrentRevisionID)
	if err != nil {
		return domain.Project{}, domain.ProjectRevision{}, err
	}
	if source.Status != domain.RevisionDraft && source.Status != domain.RevisionValidated {
		return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("%w: current revision cannot be forked from %s", domain.ErrConflict, source.Status)
	}

	cues, err := s.ListCues(ctx, source.ID)
	if err != nil {
		return domain.Project{}, domain.ProjectRevision{}, err
	}
	inputs, err := s.ListInputs(ctx, source.ID)
	if err != nil {
		return domain.Project{}, domain.ProjectRevision{}, err
	}
	outputs, err := s.ListOutputs(ctx, source.ID)
	if err != nil {
		return domain.Project{}, domain.ProjectRevision{}, err
	}
	routes, err := s.ListRoutes(ctx, source.ID)
	if err != nil {
		return domain.Project{}, domain.ProjectRevision{}, err
	}
	executionEnvironments, err := s.ListExecutionEnvironmentManifests(ctx, source.ID)
	if err != nil {
		return domain.Project{}, domain.ProjectRevision{}, err
	}
	executionEnvironmentSnapshots := make(map[string][]ExecutionEnvironmentSnapshot, len(executionEnvironments))
	for _, environment := range executionEnvironments {
		snapshots, err := s.ListExecutionEnvironmentSnapshots(ctx, environment.ID)
		if err != nil {
			return domain.Project{}, domain.ProjectRevision{}, err
		}
		executionEnvironmentSnapshots[environment.ID] = snapshots
	}

	executionEnvironmentRebuildPlans := make(map[string]*ExecutionEnvironmentRebuildPlan, len(executionEnvironments))
	for _, environment := range executionEnvironments {
		plan, err := s.GetExecutionEnvironmentRebuildPlan(ctx, environment.ID)
		if err != nil {
			if err == domain.ErrNotFound {
				continue
			}
			return domain.Project{}, domain.ProjectRevision{}, err
		}
		copy := plan
		executionEnvironmentRebuildPlans[environment.ID] = &copy
	}

	newProjectID, err := stageid.New()
    if err != nil { return domain.Project{}, domain.ProjectRevision{}, err }
    newRevisionID, err := stageid.New()
	if err != nil {
		return domain.Project{}, domain.ProjectRevision{}, err
	}
	now := s.clock.Now().UTC()
	nowUS := clock.UnixMicros(now)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("begin draft fork: %w", err)
	}
	defer tx.Rollback()

    aliases, err := duplicateSourceAliasesTx(ctx, tx, sourceProjectID)
    if err != nil { return domain.Project{}, domain.ProjectRevision{}, err }

	if _, err := tx.ExecContext(ctx, "INSERT INTO projects (project_id,name,description,lifecycle_state,current_revision_id,default_venue_profile_id,created_at_us,updated_at_us) VALUES (?,?,?,'ACTIVE',NULL,?,?,?)", newProjectID, newName, project.Description, project.DefaultVenueProfileID, nowUS, nowUS); err != nil { return domain.Project{}, domain.ProjectRevision{}, err }
    var currentRevisionID string
	if err := tx.QueryRowContext(ctx, `SELECT current_revision_id FROM projects WHERE project_id = ?`, sourceProjectID).Scan(&currentRevisionID); err != nil {
		if err == sql.ErrNoRows {
			return domain.Project{}, domain.ProjectRevision{}, domain.ErrNotFound
		}
		return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("recheck project revision: %w", err)
	}
	if currentRevisionID != source.ID {
		return domain.Project{}, domain.ProjectRevision{}, domain.ErrConflict
	}
	nextNumber := int64(1)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO project_revisions
		(revision_id, project_id, revision_number, status, parent_revision_id, created_at_us, created_by, change_note)
		VALUES (?, ?, ?, 'DRAFT', ?, ?, ?, ?)
	`, newRevisionID, newProjectID, nextNumber, nil, nowUS, createdBy, "Duplicated from "+sourceProjectID); err != nil {
		return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("insert draft successor: %w", err)
	}

    roleIDs, err := duplicateMachineRolesTx(ctx, tx, sourceProjectID, newProjectID, nowUS)
    if err != nil { return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("duplicate roles: %w", err) }

	for _, environment := range executionEnvironments {
		newEnvironmentID, err := stageid.New()
		if err != nil {
			return domain.Project{}, domain.ProjectRevision{}, err
		}
		canonical, err := executionenv.CanonicalBytes(environment.Manifest)
		if err != nil {
			return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("clone execution environment canonical manifest: %w", err)
		}
		var machineRoleID any
		if environment.MachineRoleID != nil {
            mapped, ok := roleIDs[*environment.MachineRoleID]
            if !ok { return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("%w: unrecognized Machine Role in execution environment", domain.ErrConflict) }
            machineRoleID = mapped
        }
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO execution_environment_manifests (
				environment_manifest_id, revision_id, environment_key, adapter_key, application_key,
				manifest_json, content_sha256, created_by, created_at_us, machine_role_id
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			newEnvironmentID, newRevisionID, environment.Manifest.EnvironmentKey,
			environment.Manifest.AdapterKey, environment.Manifest.Application.Key,
			string(canonical), environment.ContentSHA256, createdBy, nowUS, machineRoleID,
		); err != nil {
			return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("clone execution environment: %w", err)
		}
		snapshotIDMap := make(map[string]string, len(executionEnvironmentSnapshots[environment.ID]))
		for _, environmentSnapshot := range executionEnvironmentSnapshots[environment.ID] {
			newEnvironmentSnapshotID, err := stageid.New()
			if err != nil {
				return domain.Project{}, domain.ProjectRevision{}, err
			}
			snapshotIDMap[environmentSnapshot.ID] = newEnvironmentSnapshotID
			snapshotCanonical, err := executionenv.SnapshotCanonicalBytes(environmentSnapshot.Snapshot)
			if err != nil {
				return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("clone execution environment snapshot canonical data: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO execution_environment_snapshots (
					environment_snapshot_id, environment_manifest_id, revision_id, source_manifest_sha256,
					snapshot_json, content_sha256, created_by, created_at_us
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				newEnvironmentSnapshotID, newEnvironmentID, newRevisionID,
				environmentSnapshot.Snapshot.SourceManifestSHA256, string(snapshotCanonical),
				environmentSnapshot.ContentSHA256, createdBy, nowUS,
			); err != nil {
				return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("clone execution environment snapshot: %w", err)
			}
		}

		if sourcePlan := executionEnvironmentRebuildPlans[environment.ID]; sourcePlan != nil {
			newSourceSnapshotID, ok := snapshotIDMap[sourcePlan.SourceSnapshotID]
			if !ok {
				return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("%w: assisted rebuild plan source snapshot missing during draft fork", domain.ErrConflict)
			}
			var matchedSnapshot *ExecutionEnvironmentSnapshot
			for i := range executionEnvironmentSnapshots[environment.ID] {
				if executionEnvironmentSnapshots[environment.ID][i].ID == sourcePlan.SourceSnapshotID {
					matchedSnapshot = &executionEnvironmentSnapshots[environment.ID][i]
					break
				}
			}
			if matchedSnapshot == nil {
				return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("%w: assisted rebuild plan source snapshot unavailable during draft fork", domain.ErrConflict)
			}
			planCanonical, err := executionenv.AssistedRebuildPlanCanonicalBytes(sourcePlan.Plan, matchedSnapshot.Snapshot)
			if err != nil {
				return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("clone assisted rebuild plan canonical data: %w", err)
			}
			newPlanID, err := stageid.New()
			if err != nil {
				return domain.Project{}, domain.ProjectRevision{}, err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO execution_environment_rebuild_plans (
					rebuild_plan_id, environment_manifest_id, revision_id, source_snapshot_id,
					source_snapshot_sha256, plan_json, content_sha256,
					created_by, created_at_us, updated_by, updated_at_us
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				newPlanID, newEnvironmentID, newRevisionID, newSourceSnapshotID,
				sourcePlan.Plan.SourceSnapshotSHA256, string(planCanonical), sourcePlan.ContentSHA256,
				createdBy, nowUS, createdBy, nowUS,
			); err != nil {
				return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("clone assisted rebuild plan: %w", err)
			}
		}
	}

	cueIDs := make(map[string]string, len(cues))
	for _, cue := range cues {
		newCueID, err := stageid.New()
		if err != nil {
			return domain.Project{}, domain.ProjectRevision{}, err
		}
		cueIDs[cue.ID] = newCueID
	}
	for _, cue := range cues {
		newCueID := cueIDs[cue.ID]
		policy, err := remapForkedCueExecutionPolicy(cue.ExecutionPolicy, cueIDs)
		if err != nil {
			return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("clone cue execution policy: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO cues
			(cue_id, revision_id, display_label, name, order_index, cue_type, criticality, enabled, execution_policy_json, notes_summary)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, newCueID, newRevisionID, cue.DisplayLabel, cue.Name, cue.OrderIndex, cue.CueType,
			cue.Criticality, boolInt(cue.Enabled), string(policy), cue.NotesSummary); err != nil {
			return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("clone cue: %w", err)
		}
		for _, action := range cue.Actions {
			newActionID, err := stageid.New()
			if err != nil {
				return domain.Project{}, domain.ProjectRevision{}, err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO actions
				(action_id, cue_id, order_index, execution_mode, target_ref, capability_key,
				 parameters_json, timeout_policy_json, error_policy_json, priority_class, enabled)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, newActionID, newCueID, action.OrderIndex, action.ExecutionMode, action.TargetRef,
				action.CapabilityKey, string(action.Parameters), string(action.TimeoutPolicy),
				string(action.ErrorPolicy), action.PriorityClass, boolInt(action.Enabled)); err != nil {
				return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("clone action: %w", err)
			}
		}
	}

	inputIDs := make(map[string]string, len(inputs))
	for _, input := range inputs {
		newID, err := stageid.New()
		if err != nil {
			return domain.Project{}, domain.ProjectRevision{}, err
		}
		inputIDs[input.ID] = newID
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO input_definitions
			(input_id, revision_id, name, source_ref, event_type, value_schema_json, enabled)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, newID, newRevisionID, input.Name, input.SourceRef, input.EventType, string(input.ValueSchema), boolInt(input.Enabled)); err != nil {
			return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("clone input: %w", err)
		}
	}

	outputIDs := make(map[string]string, len(outputs))
	for _, output := range outputs {
		newID, err := stageid.New()
		if err != nil {
			return domain.Project{}, domain.ProjectRevision{}, err
		}
		outputIDs[output.ID] = newID
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO output_definitions
			(output_id, revision_id, name, target_ref, capability_key, value_schema_json, criticality)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, newID, newRevisionID, output.Name, output.TargetRef, output.CapabilityKey,
			string(output.ValueSchema), output.Criticality); err != nil {
			return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("clone output: %w", err)
		}
	}

	for _, route := range routes {
		newRouteID, err := stageid.New()
		if err != nil {
			return domain.Project{}, domain.ProjectRevision{}, err
		}
		newInputID, ok := inputIDs[route.InputID]
		if !ok {
			return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("%w: route input missing during draft fork", domain.ErrConflict)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO routes
			(route_id, revision_id, name, input_id, condition_definition_json, transform_definition_json,
			 delay_ms, debounce_ms, priority_class, error_policy_json, enabled)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, newRouteID, newRevisionID, route.Name, newInputID, string(route.ConditionDefinition),
			string(route.TransformDefinition), route.DelayMS, route.DebounceMS, route.PriorityClass,
			string(route.ErrorPolicy), boolInt(route.Enabled)); err != nil {
			return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("clone route: %w", err)
		}
		for _, action := range route.Actions {
			newRouteActionID, err := stageid.New()
			if err != nil {
				return domain.Project{}, domain.ProjectRevision{}, err
			}
			var outputID, cueID any
			if action.OutputID != nil {
				mapped, ok := outputIDs[*action.OutputID]
				if !ok {
					return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("%w: route output missing during draft fork", domain.ErrConflict)
				}
				outputID = mapped
			}
			if action.CueID != nil {
				mapped, ok := cueIDs[*action.CueID]
				if !ok {
					return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("%w: route cue missing during draft fork", domain.ErrConflict)
				}
				cueID = mapped
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO route_actions
				(route_action_id, route_id, order_index, output_id, cue_id, parameters_json)
				VALUES (?, ?, ?, ?, ?, ?)
			`, newRouteActionID, newRouteID, action.OrderIndex, outputID, cueID, string(action.Parameters)); err != nil {
				return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("clone route action: %w", err)
			}
		}
	}

	if _, err := tx.ExecContext(ctx, `UPDATE projects SET current_revision_id = ?, updated_at_us = ? WHERE project_id = ?`, newRevisionID, nowUS, newProjectID); err != nil {
		return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("activate draft successor: %w", err)
	}
	for _, alias := range aliases {
        aliasID, err := stageid.New()
        if err != nil { return domain.Project{}, domain.ProjectRevision{}, err }
        if _, err := tx.ExecContext(ctx, "INSERT INTO project_device_aliases (alias_id,project_id,logical_name,logical_type,target_ref,group_name,project_config_json) VALUES (?,?,?,?,?,?,?)", aliasID, newProjectID, alias.LogicalName, alias.LogicalType, alias.TargetRef, alias.GroupName, string(alias.ProjectConfig)); err != nil {
            return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("duplicate alias: %w", err)
        }
    }
    if err := duplicateProjectExtrasTx(ctx, tx, sourceProjectID, newProjectID, source.ID, newRevisionID, createdBy, nowUS, cueIDs, roleIDs); err != nil {
        return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("duplicate Project authoring extras: %w", err)
    }
    if err := tx.Commit(); err != nil {
		return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("commit draft fork: %w", err)
	}
return domain.Project{ID: newProjectID, Name: newName, Description: project.Description, LifecycleState: domain.ProjectActive, CurrentRevisionID: newRevisionID, DefaultVenueProfileID: project.DefaultVenueProfileID, CreatedAt: now, UpdatedAt: now}, domain.ProjectRevision{ID: newRevisionID, ProjectID: newProjectID, RevisionNumber: 1, Status: domain.RevisionDraft, CreatedAt: now, CreatedBy: createdBy, ChangeNote: "Duplicated from " + sourceProjectID}, nil
}
