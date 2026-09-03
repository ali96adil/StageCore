package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

type TemplateDraftGraph struct {
	Name        string
	Description string
	CreatedBy   string
	Targets     []TemplateDraftTarget
	Cues        []TemplateDraftCue
	Inputs      []TemplateDraftInput
	Outputs     []TemplateDraftOutput
	Routes      []TemplateDraftRoute
}

type TemplateDraftTarget struct {
	Key           string
	LogicalName   string
	LogicalType   string
	TargetRef     string
	GroupName     string
	Configuration json.RawMessage
}

type TemplateDraftCue struct {
	Key             string
	DisplayLabel    string
	Name            string
	OrderIndex      int
	CueType         string
	Criticality     string
	Enabled         bool
	ExecutionPolicy json.RawMessage
	NotesSummary    string
	Actions         []TemplateDraftAction
}

type TemplateDraftAction struct {
	Key            string
	OrderIndex     int
	ExecutionMode  string
	TargetKey      string
	CapabilityKey  string
	Parameters     json.RawMessage
	TimeoutPolicy  json.RawMessage
	ErrorPolicy    json.RawMessage
	PriorityClass  string
	Enabled        bool
}

type TemplateDraftInput struct {
	Key         string
	Name        string
	SourceRef   string
	EventType   string
	ValueSchema json.RawMessage
	Enabled     bool
}

type TemplateDraftOutput struct {
	Key           string
	Name          string
	TargetKey     string
	CapabilityKey string
	ValueSchema   json.RawMessage
	Criticality   string
}

type TemplateDraftRoute struct {
	Key                  string
	Name                 string
	InputKey             string
	ConditionDefinition  json.RawMessage
	TransformDefinition  json.RawMessage
	DelayMS              *int64
	DebounceMS           *int64
	PriorityClass        string
	ErrorPolicy          json.RawMessage
	Enabled              bool
	Actions              []TemplateDraftRouteAction
}

type TemplateDraftRouteAction struct {
	Key        string
	OrderIndex int
	OutputKey  string
	CueKey     string
	Parameters json.RawMessage
}

type TemplateDraftResult struct {
	ProjectID  string
	RevisionID string
}

func (s *Store) MaterializeTemplateDraft(ctx context.Context, graph TemplateDraftGraph) (TemplateDraftResult, error) {
	if s == nil || s.db == nil {
		return TemplateDraftResult{}, fmt.Errorf("Store is unavailable")
	}
	graph.Name = strings.TrimSpace(graph.Name)
	graph.CreatedBy = strings.TrimSpace(graph.CreatedBy)
	if graph.Name == "" || len(graph.Name) > 160 || graph.CreatedBy == "" || len(graph.CreatedBy) > 256 || len(graph.Description) > 500 {
		return TemplateDraftResult{}, fmt.Errorf("%w: template Project metadata is invalid", domain.ErrInvalidInput)
	}
	if len(graph.Targets) > 64 || len(graph.Cues) > 1000 || len(graph.Inputs) > 256 || len(graph.Outputs) > 256 || len(graph.Routes) > 512 {
		return TemplateDraftResult{}, fmt.Errorf("%w: template Project graph exceeds limits", domain.ErrInvalidInput)
	}

	projectID, err := stageid.New()
	if err != nil {
		return TemplateDraftResult{}, err
	}
	revisionID, err := stageid.New()
	if err != nil {
		return TemplateDraftResult{}, err
	}

	targetIDs := map[string]string{}
	targetRefs := map[string]string{}
	for _, target := range graph.Targets {
		key := strings.TrimSpace(target.Key)
		if key == "" || targetIDs[key] != "" || strings.TrimSpace(target.LogicalName) == "" || strings.TrimSpace(target.LogicalType) == "" {
			return TemplateDraftResult{}, fmt.Errorf("%w: invalid or duplicate template target %q", domain.ErrInvalidInput, key)
		}
		id, err := stageid.New()
		if err != nil {
			return TemplateDraftResult{}, err
		}
		targetIDs[key] = id
		targetRefs[key] = strings.TrimSpace(target.LogicalName)
	}
	cueIDs := map[string]string{}
	for _, cue := range graph.Cues {
		key := strings.TrimSpace(cue.Key)
		if key == "" || cueIDs[key] != "" || strings.TrimSpace(cue.Name) == "" {
			return TemplateDraftResult{}, fmt.Errorf("%w: invalid or duplicate template cue %q", domain.ErrInvalidInput, key)
		}
		id, err := stageid.New()
		if err != nil {
			return TemplateDraftResult{}, err
		}
		cueIDs[key] = id
	}
	inputIDs := map[string]string{}
	for _, input := range graph.Inputs {
		key := strings.TrimSpace(input.Key)
		if key == "" || inputIDs[key] != "" || strings.TrimSpace(input.Name) == "" {
			return TemplateDraftResult{}, fmt.Errorf("%w: invalid or duplicate template input %q", domain.ErrInvalidInput, key)
		}
		id, err := stageid.New()
		if err != nil {
			return TemplateDraftResult{}, err
		}
		inputIDs[key] = id
	}
	outputIDs := map[string]string{}
	for _, output := range graph.Outputs {
		key := strings.TrimSpace(output.Key)
		if key == "" || outputIDs[key] != "" || strings.TrimSpace(output.Name) == "" || targetRefs[strings.TrimSpace(output.TargetKey)] == "" {
			return TemplateDraftResult{}, fmt.Errorf("%w: invalid template output %q", domain.ErrInvalidInput, key)
		}
		id, err := stageid.New()
		if err != nil {
			return TemplateDraftResult{}, err
		}
		outputIDs[key] = id
	}
	routeIDs := map[string]string{}
	for _, route := range graph.Routes {
		key := strings.TrimSpace(route.Key)
		if key == "" || routeIDs[key] != "" || strings.TrimSpace(route.Name) == "" || inputIDs[strings.TrimSpace(route.InputKey)] == "" {
			return TemplateDraftResult{}, fmt.Errorf("%w: invalid template route %q", domain.ErrInvalidInput, key)
		}
		id, err := stageid.New()
		if err != nil {
			return TemplateDraftResult{}, err
		}
		routeIDs[key] = id
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TemplateDraftResult{}, fmt.Errorf("begin template materialization: %w", err)
	}
	defer tx.Rollback()

	var activeShows int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE status = 'ACTIVE' AND session_type = 'SHOW'`).Scan(&activeShows); err != nil {
		return TemplateDraftResult{}, fmt.Errorf("check active SHOW before template materialization: %w", err)
	}
	if activeShows != 0 {
		return TemplateDraftResult{}, fmt.Errorf("%w: template materialization is blocked while SHOW is active", domain.ErrShowConfigurationLocked)
	}

	nowUS := clock.UnixMicros(s.clock.Now().UTC())
	if _, err := tx.ExecContext(ctx, `INSERT INTO projects
		(project_id, name, description, lifecycle_state, current_revision_id, created_at_us, updated_at_us)
		VALUES (?, ?, ?, 'ACTIVE', NULL, ?, ?)`, projectID, graph.Name, graph.Description, nowUS, nowUS); err != nil {
		return TemplateDraftResult{}, fmt.Errorf("insert template Project: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO project_revisions
		(revision_id, project_id, revision_number, status, parent_revision_id, created_at_us, created_by, change_note)
		VALUES (?, ?, 1, 'DRAFT', NULL, ?, ?, 'Created from Show Template')`, revisionID, projectID, nowUS, graph.CreatedBy); err != nil {
		return TemplateDraftResult{}, fmt.Errorf("insert template draft revision: %w", err)
	}

	for _, target := range graph.Targets {
		cfg, err := normalizeJSON(target.Configuration, "{}")
		if err != nil {
			return TemplateDraftResult{}, fmt.Errorf("template target %s configuration: %w", target.Key, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_device_aliases
			(alias_id, project_id, logical_name, logical_type, target_ref, group_name, project_config_json)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, targetIDs[target.Key], projectID, strings.TrimSpace(target.LogicalName), strings.TrimSpace(target.LogicalType), strings.TrimSpace(target.TargetRef), strings.TrimSpace(target.GroupName), cfg); err != nil {
			return TemplateDraftResult{}, fmt.Errorf("insert template target %s: %w", target.Key, err)
		}
	}

	for _, cue := range graph.Cues {
		policy, err := normalizeJSON(cue.ExecutionPolicy, "{}")
		if err != nil {
			return TemplateDraftResult{}, fmt.Errorf("template cue %s policy: %w", cue.Key, err)
		}
		cueType := strings.TrimSpace(cue.CueType)
		if cueType == "" { cueType = "STANDARD" }
		criticality := strings.TrimSpace(cue.Criticality)
		if criticality == "" { criticality = "NORMAL" }
		if _, err := tx.ExecContext(ctx, `INSERT INTO cues
			(cue_id, revision_id, display_label, name, order_index, cue_type, criticality, enabled, execution_policy_json, notes_summary)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, cueIDs[cue.Key], revisionID, cue.DisplayLabel, strings.TrimSpace(cue.Name), cue.OrderIndex, cueType, criticality, boolInt(cue.Enabled), policy, cue.NotesSummary); err != nil {
			return TemplateDraftResult{}, fmt.Errorf("insert template cue %s: %w", cue.Key, err)
		}
		actionKeys := map[string]bool{}
		for _, action := range cue.Actions {
			if actionKeys[action.Key] || targetRefs[action.TargetKey] == "" {
				return TemplateDraftResult{}, fmt.Errorf("%w: invalid action %q in template cue %q", domain.ErrInvalidInput, action.Key, cue.Key)
			}
			actionKeys[action.Key] = true
			actionID, err := stageid.New()
			if err != nil { return TemplateDraftResult{}, err }
			params, err := normalizeJSON(action.Parameters, "{}")
			if err != nil { return TemplateDraftResult{}, fmt.Errorf("template action parameters: %w", err) }
			timeoutPolicy, err := normalizeJSON(action.TimeoutPolicy, "{}")
			if err != nil { return TemplateDraftResult{}, fmt.Errorf("template action timeout policy: %w", err) }
			errorPolicy, err := normalizeJSON(action.ErrorPolicy, "{}")
			if err != nil { return TemplateDraftResult{}, fmt.Errorf("template action error policy: %w", err) }
			executionMode := strings.TrimSpace(action.ExecutionMode); if executionMode == "" { executionMode = "SEQUENTIAL" }
			priority := strings.TrimSpace(action.PriorityClass); if priority == "" { priority = "P1" }
			if _, err := tx.ExecContext(ctx, `INSERT INTO actions
				(action_id, cue_id, order_index, execution_mode, target_ref, capability_key, parameters_json, timeout_policy_json, error_policy_json, priority_class, enabled)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, actionID, cueIDs[cue.Key], action.OrderIndex, executionMode, targetRefs[action.TargetKey], strings.TrimSpace(action.CapabilityKey), params, timeoutPolicy, errorPolicy, priority, boolInt(action.Enabled)); err != nil {
				return TemplateDraftResult{}, fmt.Errorf("insert template action %s: %w", action.Key, err)
			}
		}
	}

	for _, input := range graph.Inputs {
		schema, err := normalizeJSON(input.ValueSchema, "{}")
		if err != nil { return TemplateDraftResult{}, fmt.Errorf("template input %s schema: %w", input.Key, err) }
		if _, err := tx.ExecContext(ctx, `INSERT INTO input_definitions
			(input_id, revision_id, name, source_ref, event_type, value_schema_json, enabled)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, inputIDs[input.Key], revisionID, strings.TrimSpace(input.Name), strings.TrimSpace(input.SourceRef), strings.TrimSpace(input.EventType), schema, boolInt(input.Enabled)); err != nil {
			return TemplateDraftResult{}, fmt.Errorf("insert template input %s: %w", input.Key, err)
		}
	}
	for _, output := range graph.Outputs {
		schema, err := normalizeJSON(output.ValueSchema, "{}")
		if err != nil { return TemplateDraftResult{}, fmt.Errorf("template output %s schema: %w", output.Key, err) }
		criticality := strings.TrimSpace(output.Criticality); if criticality == "" { criticality = "NORMAL" }
		if _, err := tx.ExecContext(ctx, `INSERT INTO output_definitions
			(output_id, revision_id, name, target_ref, capability_key, value_schema_json, criticality)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, outputIDs[output.Key], revisionID, strings.TrimSpace(output.Name), targetRefs[output.TargetKey], strings.TrimSpace(output.CapabilityKey), schema, criticality); err != nil {
			return TemplateDraftResult{}, fmt.Errorf("insert template output %s: %w", output.Key, err)
		}
	}

	for _, route := range graph.Routes {
		condition, err := normalizeJSON(route.ConditionDefinition, "null")
		if err != nil { return TemplateDraftResult{}, fmt.Errorf("template route %s condition: %w", route.Key, err) }
		transform, err := normalizeJSON(route.TransformDefinition, "null")
		if err != nil { return TemplateDraftResult{}, fmt.Errorf("template route %s transform: %w", route.Key, err) }
		errorPolicy, err := normalizeJSON(route.ErrorPolicy, "{}")
		if err != nil { return TemplateDraftResult{}, fmt.Errorf("template route %s error policy: %w", route.Key, err) }
		priority := strings.TrimSpace(route.PriorityClass); if priority == "" { priority = "P2" }
		if _, err := tx.ExecContext(ctx, `INSERT INTO routes
			(route_id, revision_id, name, input_id, condition_definition_json, transform_definition_json, delay_ms, debounce_ms, priority_class, error_policy_json, enabled)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, routeIDs[route.Key], revisionID, strings.TrimSpace(route.Name), inputIDs[route.InputKey], condition, transform, route.DelayMS, route.DebounceMS, priority, errorPolicy, boolInt(route.Enabled)); err != nil {
			return TemplateDraftResult{}, fmt.Errorf("insert template route %s: %w", route.Key, err)
		}
		actionKeys := map[string]bool{}
		for _, action := range route.Actions {
			if actionKeys[action.Key] || (action.OutputKey == "") == (action.CueKey == "") {
				return TemplateDraftResult{}, fmt.Errorf("%w: invalid route action %q", domain.ErrInvalidInput, action.Key)
			}
			actionKeys[action.Key] = true
			var outputID, cueID any
			if action.OutputKey != "" {
				id := outputIDs[action.OutputKey]; if id == "" { return TemplateDraftResult{}, fmt.Errorf("%w: unknown output %q", domain.ErrInvalidInput, action.OutputKey) }; outputID = id
			}
			if action.CueKey != "" {
				id := cueIDs[action.CueKey]; if id == "" { return TemplateDraftResult{}, fmt.Errorf("%w: unknown cue %q", domain.ErrInvalidInput, action.CueKey) }; cueID = id
			}
			params, err := normalizeJSON(action.Parameters, "{}")
			if err != nil { return TemplateDraftResult{}, fmt.Errorf("template route action parameters: %w", err) }
			actionID, err := stageid.New(); if err != nil { return TemplateDraftResult{}, err }
			if _, err := tx.ExecContext(ctx, `INSERT INTO route_actions
				(route_action_id, route_id, order_index, output_id, cue_id, parameters_json)
				VALUES (?, ?, ?, ?, ?, ?)`, actionID, routeIDs[route.Key], action.OrderIndex, outputID, cueID, params); err != nil {
				return TemplateDraftResult{}, fmt.Errorf("insert template route action %s: %w", action.Key, err)
			}
		}
	}

	if _, err := tx.ExecContext(ctx, `UPDATE projects SET current_revision_id = ? WHERE project_id = ?`, revisionID, projectID); err != nil {
		return TemplateDraftResult{}, fmt.Errorf("bind template current revision: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return TemplateDraftResult{}, fmt.Errorf("commit template materialization: %w", err)
	}
	return TemplateDraftResult{ProjectID: projectID, RevisionID: revisionID}, nil
}
