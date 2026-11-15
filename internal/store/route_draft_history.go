package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/domain"
	sqlite "modernc.org/sqlite"
)

const (
	routeDraftHistoryLimit       = 50
	routeDraftHistorySystemActor = "_stagecore_system_"
)

type routeDraftHistoryActorKey struct{}

// WithRouteDraftHistoryActor scopes routing Undo/Redo to the authenticated
// local editor. Store callers without an actor use an internal history scope.
func WithRouteDraftHistoryActor(ctx context.Context, actorID string) context.Context {
	return context.WithValue(ctx, routeDraftHistoryActorKey{}, strings.TrimSpace(actorID))
}

func routeDraftHistoryActor(ctx context.Context) string {
	actor, _ := ctx.Value(routeDraftHistoryActorKey{}).(string)
	if actor = strings.TrimSpace(actor); actor == "" {
		return routeDraftHistorySystemActor
	}
	return actor
}

// RouteDraftHistoryStatus contains only availability and operation labels.
// Route definitions remain available through the normal configuration API.
type RouteDraftHistoryStatus struct {
	ProjectID     string `json:"project_id"`
	RevisionID    string `json:"revision_id"`
	CanUndo       bool   `json:"can_undo"`
	CanRedo       bool   `json:"can_redo"`
	UndoOperation string `json:"undo_operation,omitempty"`
	RedoOperation string `json:"redo_operation,omitempty"`
}

type routeDraftHistoryDelta struct {
	Version    int           `json:"version"`
	BeforeHash string        `json:"before_sha256"`
	AfterHash  string        `json:"after_sha256"`
	Before     *domain.Route `json:"before_route,omitempty"`
	After      *domain.Route `json:"after_route,omitempty"`
}

func readRouteTx(ctx context.Context, tx *sql.Tx, revisionID, routeID string) (*domain.Route, error) {
	var route domain.Route
	var condition, transform, errorPolicy, priority string
	var delay, debounce sql.NullInt64
	var enabled int
	err := tx.QueryRowContext(ctx, `
		SELECT route_id, revision_id, name, input_id, condition_definition_json,
		       transform_definition_json, delay_ms, debounce_ms, priority_class,
		       error_policy_json, enabled
		FROM routes WHERE route_id = ? AND revision_id = ?
	`, routeID, revisionID).Scan(
		&route.ID, &route.RevisionID, &route.Name, &route.InputID, &condition,
		&transform, &delay, &debounce, &priority, &errorPolicy, &enabled,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read Route for Draft history: %w", err)
	}
	if delay.Valid {
		value := delay.Int64
		route.DelayMS = &value
	}
	if debounce.Valid {
		value := debounce.Int64
		route.DebounceMS = &value
	}
	route.ConditionDefinition = json.RawMessage(condition)
	route.TransformDefinition = json.RawMessage(transform)
	route.PriorityClass = domain.PriorityClass(priority)
	route.ErrorPolicy = json.RawMessage(errorPolicy)
	route.Enabled = enabled == 1
	route.Actions = []domain.RouteAction{}
	rows, err := tx.QueryContext(ctx, `
		SELECT route_action_id, route_id, order_index, output_id, cue_id, parameters_json
		FROM route_actions WHERE route_id = ? ORDER BY order_index, route_action_id
	`, route.ID)
	if err != nil {
		return nil, fmt.Errorf("read Route actions for Draft history: %w", err)
	}
	for rows.Next() {
		var action domain.RouteAction
		var outputID, cueID sql.NullString
		var parameters string
		if err := rows.Scan(&action.ID, &action.RouteID, &action.OrderIndex, &outputID, &cueID, &parameters); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan Route action for Draft history: %w", err)
		}
		if outputID.Valid {
			value := outputID.String
			action.OutputID = &value
		}
		if cueID.Valid {
			value := cueID.String
			action.CueID = &value
		}
		action.Parameters = json.RawMessage(parameters)
		route.Actions = append(route.Actions, action)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate Route actions for Draft history: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close Route action history rows: %w", err)
	}
	return &route, nil
}

func routeDraftHistoryHash(route *domain.Route) (string, error) {
	if route == nil {
		return "absent", nil
	}
	encoded, err := json.Marshal(route)
	if err != nil {
		return "", fmt.Errorf("encode Route Draft history state: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func recordRouteDraftHistoryTx(ctx context.Context, tx *sql.Tx, revisionID, operation, routeID string, before, after *domain.Route, nowUS int64) error {
	beforeHash, err := routeDraftHistoryHash(before)
	if err != nil {
		return err
	}
	afterHash, err := routeDraftHistoryHash(after)
	if err != nil {
		return err
	}
	if beforeHash == afterHash {
		return nil
	}
	actor := routeDraftHistoryActor(ctx)
	if len(actor) > 256 {
		return fmt.Errorf("%w: Route history actor is too long", domain.ErrInvalidInput)
	}
	encoded, err := json.Marshal(routeDraftHistoryDelta{
		Version: 1, BeforeHash: beforeHash, AfterHash: afterHash, Before: before, After: after,
	})
	if err != nil {
		return fmt.Errorf("encode Route Draft history delta: %w", err)
	}
	if len(encoded) > 4<<20 {
		return fmt.Errorf("%w: Route Draft history entry exceeds 4 MiB", domain.ErrInvalidInput)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO route_draft_history_state (revision_id, actor_id)
		VALUES (?, ?)
	`, revisionID, actor); err != nil {
		if routeDraftHistorySchemaMissing(err) {
			// Migration tests deliberately exercise old schemas. Product startup
			// always applies all migrations before handlers are registered.
			return nil
		}
		return fmt.Errorf("initialize Route Draft history: %w", err)
	}
	var nextSequence, cursor int64
	if err := tx.QueryRowContext(ctx, `
		SELECT next_sequence, cursor_sequence FROM route_draft_history_state
		WHERE revision_id = ? AND actor_id = ?
	`, revisionID, actor).Scan(&nextSequence, &cursor); err != nil {
		return fmt.Errorf("read Route Draft history cursor: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM route_draft_history_events
		WHERE revision_id = ? AND actor_id = ? AND sequence > ?
	`, revisionID, actor, cursor); err != nil {
		return fmt.Errorf("discard Route Draft redo history: %w", err)
	}
	// Reuse the first discarded sequence when an editor branches after Undo.
	// Keeping next_sequence beyond the removed Redo branch leaves a cursor gap,
	// which would make the retained preceding operation unreachable by Undo.
	nextSequence = cursor + 1
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO route_draft_history_events
		(revision_id, actor_id, sequence, operation, route_id, created_at_us, delta_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, revisionID, actor, nextSequence, operation, routeID, nowUS, string(encoded)); err != nil {
		return fmt.Errorf("append Route Draft history: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE route_draft_history_state SET cursor_sequence = ?, next_sequence = ?
		WHERE revision_id = ? AND actor_id = ?
	`, nextSequence, nextSequence+1, revisionID, actor); err != nil {
		return fmt.Errorf("advance Route Draft history cursor: %w", err)
	}
	oldest := nextSequence - routeDraftHistoryLimit
	if oldest > 0 {
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM route_draft_history_events
			WHERE revision_id = ? AND actor_id = ? AND sequence <= ?
		`, revisionID, actor, oldest); err != nil {
			return fmt.Errorf("bound Route Draft history: %w", err)
		}
	}
	return nil
}

func routeDraftHistorySchemaMissing(err error) bool {
	var sqliteErr *sqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code() == 1 &&
		strings.Contains(sqliteErr.Error(), "no such table: route_draft_history_state")
}

func (s *Store) RouteDraftHistory(ctx context.Context, projectID string) (RouteDraftHistoryStatus, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return RouteDraftHistoryStatus{}, fmt.Errorf("%w: Project is required", domain.ErrInvalidInput)
	}
	project, err := s.GetProject(ctx, projectID)
	if err != nil {
		return RouteDraftHistoryStatus{}, err
	}
	status := RouteDraftHistoryStatus{ProjectID: projectID, RevisionID: project.CurrentRevisionID}
	revision, err := s.GetRevision(ctx, project.CurrentRevisionID)
	if err != nil {
		return RouteDraftHistoryStatus{}, err
	}
	if revision.Status != domain.RevisionDraft {
		return status, nil
	}
	actor := routeDraftHistoryActor(ctx)
	var cursor int64
	err = s.db.QueryRowContext(ctx, `
		SELECT cursor_sequence FROM route_draft_history_state WHERE revision_id = ? AND actor_id = ?
	`, revision.ID, actor).Scan(&cursor)
	if errors.Is(err, sql.ErrNoRows) {
		return status, nil
	}
	if err != nil {
		return RouteDraftHistoryStatus{}, fmt.Errorf("read Route Draft history status: %w", err)
	}
	var operation string
	err = s.db.QueryRowContext(ctx, `
		SELECT operation FROM route_draft_history_events
		WHERE revision_id = ? AND actor_id = ? AND sequence = ?
	`, revision.ID, actor, cursor).Scan(&operation)
	if err == nil {
		status.CanUndo, status.UndoOperation = true, operation
	} else if !errors.Is(err, sql.ErrNoRows) {
		return RouteDraftHistoryStatus{}, fmt.Errorf("read Route Draft undo availability: %w", err)
	}
	err = s.db.QueryRowContext(ctx, `
		SELECT operation FROM route_draft_history_events
		WHERE revision_id = ? AND actor_id = ? AND sequence = ?
	`, revision.ID, actor, cursor+1).Scan(&operation)
	if err == nil {
		status.CanRedo, status.RedoOperation = true, operation
	} else if !errors.Is(err, sql.ErrNoRows) {
		return RouteDraftHistoryStatus{}, fmt.Errorf("read Route Draft redo availability: %w", err)
	}
	return status, nil
}

func (s *Store) UndoRouteDraft(ctx context.Context, projectID string) (RouteDraftHistoryStatus, error) {
	return s.moveRouteDraftHistory(ctx, projectID, false)
}

func (s *Store) RedoRouteDraft(ctx context.Context, projectID string) (RouteDraftHistoryStatus, error) {
	return s.moveRouteDraftHistory(ctx, projectID, true)
}

func (s *Store) moveRouteDraftHistory(ctx context.Context, projectID string, redo bool) (RouteDraftHistoryStatus, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return RouteDraftHistoryStatus{}, fmt.Errorf("%w: Project is required", domain.ErrInvalidInput)
	}
	if err := s.RequireProjectConfigurationMutable(ctx, projectID); err != nil {
		return RouteDraftHistoryStatus{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RouteDraftHistoryStatus{}, fmt.Errorf("begin Route Draft history move: %w", err)
	}
	defer tx.Rollback()
	revisionID, err := readCurrentProjectDraftTx(ctx, tx, projectID)
	if err != nil {
		return RouteDraftHistoryStatus{}, err
	}
	if err := s.ensureDraft(ctx, tx, revisionID); err != nil {
		return RouteDraftHistoryStatus{}, err
	}
	actor := routeDraftHistoryActor(ctx)
	var cursor int64
	err = tx.QueryRowContext(ctx, `
		SELECT cursor_sequence FROM route_draft_history_state WHERE revision_id = ? AND actor_id = ?
	`, revisionID, actor).Scan(&cursor)
	if errors.Is(err, sql.ErrNoRows) {
		return RouteDraftHistoryStatus{}, domain.ErrNotFound
	}
	if err != nil {
		return RouteDraftHistoryStatus{}, fmt.Errorf("read Route Draft history cursor: %w", err)
	}
	sequence := cursor
	if redo {
		sequence++
	}
	var operation, routeID, rawDelta string
	err = tx.QueryRowContext(ctx, `
		SELECT operation, route_id, delta_json FROM route_draft_history_events
		WHERE revision_id = ? AND actor_id = ? AND sequence = ?
	`, revisionID, actor, sequence).Scan(&operation, &routeID, &rawDelta)
	if errors.Is(err, sql.ErrNoRows) {
		return RouteDraftHistoryStatus{}, domain.ErrNotFound
	}
	if err != nil {
		return RouteDraftHistoryStatus{}, fmt.Errorf("read Route Draft history event: %w", err)
	}
	var delta routeDraftHistoryDelta
	if err := json.Unmarshal([]byte(rawDelta), &delta); err != nil || delta.Version != 1 {
		return RouteDraftHistoryStatus{}, fmt.Errorf("%w: invalid Route Draft history event", domain.ErrConflict)
	}
	current, err := readRouteTx(ctx, tx, revisionID, routeID)
	if err != nil {
		return RouteDraftHistoryStatus{}, err
	}
	currentHash, err := routeDraftHistoryHash(current)
	if err != nil {
		return RouteDraftHistoryStatus{}, err
	}
	expectedHash := delta.AfterHash
	target := delta.Before
	if redo {
		expectedHash, target = delta.BeforeHash, delta.After
	}
	if currentHash != expectedHash {
		return RouteDraftHistoryStatus{}, fmt.Errorf("%w: Route changed after this editor's history entry", domain.ErrConflict)
	}
	if err := s.restoreRouteDraftHistoryTx(ctx, tx, revisionID, routeID, target); err != nil {
		return RouteDraftHistoryStatus{}, err
	}
	updated, err := readRouteTx(ctx, tx, revisionID, routeID)
	if err != nil {
		return RouteDraftHistoryStatus{}, err
	}
	wantHash := delta.BeforeHash
	if redo {
		wantHash = delta.AfterHash
	}
	updatedHash, err := routeDraftHistoryHash(updated)
	if err != nil {
		return RouteDraftHistoryStatus{}, err
	}
	if updatedHash != wantHash {
		return RouteDraftHistoryStatus{}, fmt.Errorf("%w: restored Route does not match its recorded state", domain.ErrConflict)
	}
	newCursor := sequence
	if !redo {
		newCursor--
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE route_draft_history_state SET cursor_sequence = ?
		WHERE revision_id = ? AND actor_id = ? AND cursor_sequence = ?
	`, newCursor, revisionID, actor, cursor)
	if err != nil {
		return RouteDraftHistoryStatus{}, fmt.Errorf("advance Route Draft history cursor: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return RouteDraftHistoryStatus{}, fmt.Errorf("Route Draft history cursor rows affected: %w", err)
		}
		return RouteDraftHistoryStatus{}, domain.ErrConflict
	}
	if err := capability.CheckWriteAuthorization(ctx, tx); err != nil {
		return RouteDraftHistoryStatus{}, err
	}
	if err := tx.Commit(); err != nil {
		return RouteDraftHistoryStatus{}, fmt.Errorf("commit Route Draft history move: %w", err)
	}
	return s.RouteDraftHistory(ctx, projectID)
}

func (s *Store) restoreRouteDraftHistoryTx(ctx context.Context, tx *sql.Tx, revisionID, routeID string, target *domain.Route) error {
	if target == nil {
		result, err := tx.ExecContext(ctx, `DELETE FROM routes WHERE route_id = ? AND revision_id = ?`, routeID, revisionID)
		if err != nil {
			return fmt.Errorf("remove Route from Draft history: %w", err)
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			if err != nil {
				return fmt.Errorf("count removed Routes: %w", err)
			}
			return domain.ErrConflict
		}
		return nil
	}
	if target.ID != routeID || target.RevisionID != revisionID {
		return fmt.Errorf("%w: Route history identity mismatch", domain.ErrConflict)
	}
	if ok, err := s.entityBelongsToRevisionTx(ctx, tx, "input_definitions", "input_id", target.InputID, revisionID); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("%w: Route input was removed after the history entry", domain.ErrConflict)
	}
	for _, action := range target.Actions {
		if action.RouteID != routeID || action.ID == "" || (action.OutputID == nil) == (action.CueID == nil) {
			return fmt.Errorf("%w: invalid Route action in history entry", domain.ErrConflict)
		}
		if action.OutputID != nil {
			if ok, err := s.entityBelongsToRevisionTx(ctx, tx, "output_definitions", "output_id", *action.OutputID, revisionID); err != nil {
				return err
			} else if !ok {
				return fmt.Errorf("%w: Route output was removed after the history entry", domain.ErrConflict)
			}
		}
		if action.CueID != nil {
			if ok, err := s.entityBelongsToRevisionTx(ctx, tx, "cues", "cue_id", *action.CueID, revisionID); err != nil {
				return err
			} else if !ok {
				return fmt.Errorf("%w: Route Cue was removed after the history entry", domain.ErrConflict)
			}
		}
	}
	condition, err := normalizeJSON(target.ConditionDefinition, "null")
	if err != nil {
		return fmt.Errorf("%w: invalid Route condition in history entry", domain.ErrConflict)
	}
	transform, err := normalizeJSON(target.TransformDefinition, "null")
	if err != nil {
		return fmt.Errorf("%w: invalid Route transform in history entry", domain.ErrConflict)
	}
	errorPolicy, err := normalizeJSON(target.ErrorPolicy, "{}")
	if err != nil {
		return fmt.Errorf("%w: invalid Route error policy in history entry", domain.ErrConflict)
	}
	priority := target.PriorityClass
	if priority == "" {
		priority = domain.PriorityP2
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE routes SET name = ?, input_id = ?, condition_definition_json = ?, transform_definition_json = ?,
		       delay_ms = ?, debounce_ms = ?, priority_class = ?, error_policy_json = ?, enabled = ?
		WHERE route_id = ? AND revision_id = ?
	`, strings.TrimSpace(target.Name), target.InputID, condition, transform, target.DelayMS, target.DebounceMS,
		priority, errorPolicy, boolInt(target.Enabled), routeID, revisionID)
	if err != nil {
		return fmt.Errorf("restore Route history: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("count restored Route rows: %w", err)
	} else if affected == 0 {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO routes (route_id, revision_id, name, input_id, condition_definition_json,
			                    transform_definition_json, delay_ms, debounce_ms, priority_class, error_policy_json, enabled)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, routeID, revisionID, strings.TrimSpace(target.Name), target.InputID, condition, transform, target.DelayMS,
			target.DebounceMS, priority, errorPolicy, boolInt(target.Enabled)); err != nil {
			return fmt.Errorf("recreate Route from history: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM route_actions WHERE route_id = ?`, routeID); err != nil {
		return fmt.Errorf("replace Route actions from history: %w", err)
	}
	for _, action := range target.Actions {
		parameters, err := normalizeJSON(action.Parameters, "{}")
		if err != nil {
			return fmt.Errorf("%w: invalid Route action parameters in history entry", domain.ErrConflict)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO route_actions (route_action_id, route_id, order_index, output_id, cue_id, parameters_json)
			VALUES (?, ?, ?, ?, ?, ?)
		`, action.ID, routeID, action.OrderIndex, action.OutputID, action.CueID, parameters); err != nil {
			return fmt.Errorf("restore Route action from history: %w", err)
		}
	}
	return nil
}
