package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

func (s *Store) CreateInput(ctx context.Context, input domain.InputDefinition) (domain.InputDefinition, error) {
	if err := s.ensureDraft(ctx, s.db, input.RevisionID); err != nil {
		return domain.InputDefinition{}, err
	}
	if input.ID == "" {
		var err error
		input.ID, err = stageid.New()
		if err != nil {
			return domain.InputDefinition{}, err
		}
	}
	schema, err := normalizeJSON(input.ValueSchema, "{}")
	if err != nil {
		return domain.InputDefinition{}, fmt.Errorf("input value schema: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO input_definitions (input_id, revision_id, name, source_ref, event_type, value_schema_json, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, input.ID, input.RevisionID, strings.TrimSpace(input.Name), input.SourceRef, input.EventType, schema, boolInt(input.Enabled))
	if err != nil {
		return domain.InputDefinition{}, fmt.Errorf("insert input: %w", err)
	}
	input.ValueSchema = json.RawMessage(schema)
	return input, nil
}

func (s *Store) CreateOutput(ctx context.Context, output domain.OutputDefinition) (domain.OutputDefinition, error) {
	if err := s.ensureDraft(ctx, s.db, output.RevisionID); err != nil {
		return domain.OutputDefinition{}, err
	}
	if output.ID == "" {
		var err error
		output.ID, err = stageid.New()
		if err != nil {
			return domain.OutputDefinition{}, err
		}
	}
	if output.Criticality == "" {
		output.Criticality = "NORMAL"
	}
	schema, err := normalizeJSON(output.ValueSchema, "{}")
	if err != nil {
		return domain.OutputDefinition{}, fmt.Errorf("output value schema: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO output_definitions (output_id, revision_id, name, target_ref, capability_key, value_schema_json, criticality)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, output.ID, output.RevisionID, strings.TrimSpace(output.Name), output.TargetRef, output.CapabilityKey, schema, output.Criticality)
	if err != nil {
		return domain.OutputDefinition{}, fmt.Errorf("insert output: %w", err)
	}
	output.ValueSchema = json.RawMessage(schema)
	return output, nil
}

func (s *Store) ListInputs(ctx context.Context, revisionID string) ([]domain.InputDefinition, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT input_id, revision_id, name, source_ref, event_type, value_schema_json, enabled FROM input_definitions WHERE revision_id = ? ORDER BY name, input_id`, revisionID)
	if err != nil {
		return nil, fmt.Errorf("list inputs: %w", err)
	}
	defer rows.Close()
	var values []domain.InputDefinition
	for rows.Next() {
		var v domain.InputDefinition
		var schema string
		var enabled int
		if err := rows.Scan(&v.ID, &v.RevisionID, &v.Name, &v.SourceRef, &v.EventType, &schema, &enabled); err != nil {
			return nil, fmt.Errorf("scan input: %w", err)
		}
		v.ValueSchema = json.RawMessage(schema)
		v.Enabled = enabled == 1
		values = append(values, v)
	}
	return values, rows.Err()
}

func (s *Store) ListOutputs(ctx context.Context, revisionID string) ([]domain.OutputDefinition, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT output_id, revision_id, name, target_ref, capability_key, value_schema_json, criticality FROM output_definitions WHERE revision_id = ? ORDER BY name, output_id`, revisionID)
	if err != nil {
		return nil, fmt.Errorf("list outputs: %w", err)
	}
	defer rows.Close()
	var values []domain.OutputDefinition
	for rows.Next() {
		var v domain.OutputDefinition
		var schema string
		if err := rows.Scan(&v.ID, &v.RevisionID, &v.Name, &v.TargetRef, &v.CapabilityKey, &schema, &v.Criticality); err != nil {
			return nil, fmt.Errorf("scan output: %w", err)
		}
		v.ValueSchema = json.RawMessage(schema)
		values = append(values, v)
	}
	return values, rows.Err()
}

func (s *Store) CreateRouteWithActions(ctx context.Context, route domain.Route, actions []domain.RouteAction) (domain.Route, error) {
	if err := s.ensureDraft(ctx, s.db, route.RevisionID); err != nil {
		return domain.Route{}, err
	}
	if ok, err := s.entityBelongsToRevision(ctx, "input_definitions", "input_id", route.InputID, route.RevisionID); err != nil {
		return domain.Route{}, err
	} else if !ok {
		return domain.Route{}, fmt.Errorf("%w: route input does not belong to revision", domain.ErrInvalidInput)
	}
	if route.ID == "" {
		var err error
		route.ID, err = stageid.New()
		if err != nil {
			return domain.Route{}, err
		}
	}
	if route.PriorityClass == "" {
		route.PriorityClass = domain.PriorityP2
	}
	condition, err := normalizeJSON(route.ConditionDefinition, "null")
	if err != nil {
		return domain.Route{}, fmt.Errorf("route condition: %w", err)
	}
	transform, err := normalizeJSON(route.TransformDefinition, "null")
	if err != nil {
		return domain.Route{}, fmt.Errorf("route transform: %w", err)
	}
	errorPolicy, err := normalizeJSON(route.ErrorPolicy, "{}")
	if err != nil {
		return domain.Route{}, fmt.Errorf("route error policy: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Route{}, fmt.Errorf("begin create route: %w", err)
	}
	defer tx.Rollback()
	if err := s.ensureDraft(ctx, tx, route.RevisionID); err != nil {
		return domain.Route{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO routes (route_id, revision_id, name, input_id, condition_definition_json, transform_definition_json, delay_ms, debounce_ms, priority_class, error_policy_json, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		route.ID, route.RevisionID, strings.TrimSpace(route.Name), route.InputID, condition, transform, route.DelayMS, route.DebounceMS, route.PriorityClass, errorPolicy, boolInt(route.Enabled)); err != nil {
		return domain.Route{}, fmt.Errorf("insert route: %w", err)
	}

	route.Actions = make([]domain.RouteAction, 0, len(actions))
	for _, action := range actions {
		if (action.OutputID == nil) == (action.CueID == nil) {
			return domain.Route{}, fmt.Errorf("%w: route action requires exactly one output or cue", domain.ErrInvalidInput)
		}
		if action.OutputID != nil {
			ok, err := s.entityBelongsToRevisionTx(ctx, tx, "output_definitions", "output_id", *action.OutputID, route.RevisionID)
			if err != nil {
				return domain.Route{}, err
			}
			if !ok {
				return domain.Route{}, fmt.Errorf("%w: route output does not belong to revision", domain.ErrInvalidInput)
			}
		}
		if action.CueID != nil {
			ok, err := s.entityBelongsToRevisionTx(ctx, tx, "cues", "cue_id", *action.CueID, route.RevisionID)
			if err != nil {
				return domain.Route{}, err
			}
			if !ok {
				return domain.Route{}, fmt.Errorf("%w: route cue does not belong to revision", domain.ErrInvalidInput)
			}
		}
		action.RouteID = route.ID
		if action.ID == "" {
			action.ID, err = stageid.New()
			if err != nil {
				return domain.Route{}, err
			}
		}
		params, err := normalizeJSON(action.Parameters, "{}")
		if err != nil {
			return domain.Route{}, fmt.Errorf("route action parameters: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO route_actions (route_action_id, route_id, order_index, output_id, cue_id, parameters_json)
			VALUES (?, ?, ?, ?, ?, ?)`, action.ID, action.RouteID, action.OrderIndex, action.OutputID, action.CueID, params); err != nil {
			return domain.Route{}, fmt.Errorf("insert route action: %w", err)
		}
		action.Parameters = json.RawMessage(params)
		route.Actions = append(route.Actions, action)
	}
	if err := tx.Commit(); err != nil {
		return domain.Route{}, fmt.Errorf("commit create route: %w", err)
	}
	route.ConditionDefinition = json.RawMessage(condition)
	route.TransformDefinition = json.RawMessage(transform)
	route.ErrorPolicy = json.RawMessage(errorPolicy)
	return route, nil
}

func (s *Store) ListRoutes(ctx context.Context, revisionID string) ([]domain.Route, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT route_id, revision_id, name, input_id, condition_definition_json, transform_definition_json, delay_ms, debounce_ms, priority_class, error_policy_json, enabled
		FROM routes WHERE revision_id = ? ORDER BY name, route_id`, revisionID)
	if err != nil {
		return nil, fmt.Errorf("list routes: %w", err)
	}
	var routes []domain.Route
	for rows.Next() {
		var r domain.Route
		var condition, transform, priority, errorPolicy string
		var delay, debounce sql.NullInt64
		var enabled int
		if err := rows.Scan(&r.ID, &r.RevisionID, &r.Name, &r.InputID, &condition, &transform, &delay, &debounce, &priority, &errorPolicy, &enabled); err != nil {
			return nil, fmt.Errorf("scan route: %w", err)
		}
		if delay.Valid {
			v := delay.Int64
			r.DelayMS = &v
		}
		if debounce.Valid {
			v := debounce.Int64
			r.DebounceMS = &v
		}
		r.ConditionDefinition = json.RawMessage(condition)
		r.TransformDefinition = json.RawMessage(transform)
		r.PriorityClass = domain.PriorityClass(priority)
		r.ErrorPolicy = json.RawMessage(errorPolicy)
		r.Enabled = enabled == 1
		routes = append(routes, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate routes: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close route rows: %w", err)
	}
	for i := range routes {
		actions, err := s.listRouteActions(ctx, routes[i].ID)
		if err != nil {
			return nil, err
		}
		routes[i].Actions = actions
	}
	return routes, nil
}

func (s *Store) listRouteActions(ctx context.Context, routeID string) ([]domain.RouteAction, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT route_action_id, route_id, order_index, output_id, cue_id, parameters_json FROM route_actions WHERE route_id = ? ORDER BY order_index, route_action_id`, routeID)
	if err != nil {
		return nil, fmt.Errorf("list route actions: %w", err)
	}
	defer rows.Close()
	var actions []domain.RouteAction
	for rows.Next() {
		var a domain.RouteAction
		var output, cue sql.NullString
		var params string
		if err := rows.Scan(&a.ID, &a.RouteID, &a.OrderIndex, &output, &cue, &params); err != nil {
			return nil, fmt.Errorf("scan route action: %w", err)
		}
		if output.Valid {
			a.OutputID = &output.String
		}
		if cue.Valid {
			a.CueID = &cue.String
		}
		a.Parameters = json.RawMessage(params)
		actions = append(actions, a)
	}
	return actions, rows.Err()
}
