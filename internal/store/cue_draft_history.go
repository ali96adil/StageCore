package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	sqlite "modernc.org/sqlite"
)

const (
	cueDraftHistoryLimit       = 50
	cueDraftHistorySystemActor = "_stagecore_system_"
	cueDraftHistoryTempOrder   = 1000000000
)

type cueDraftHistoryActorKey struct{}

// WithCueDraftHistoryActor scopes authoring history to the authenticated local
// user. Store callers without an actor use a separate internal history scope.
func WithCueDraftHistoryActor(ctx context.Context, actorID string) context.Context {
	return context.WithValue(ctx, cueDraftHistoryActorKey{}, strings.TrimSpace(actorID))
}

func cueDraftHistoryActor(ctx context.Context) string {
	actor, _ := ctx.Value(cueDraftHistoryActorKey{}).(string)
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return cueDraftHistorySystemActor
	}
	return actor
}

// CueDraftHistoryStatus describes the current actor's bounded history for the
// current Project Draft. It never includes authored Cue payloads.
type CueDraftHistoryStatus struct {
	ProjectID     string `json:"project_id"`
	RevisionID    string `json:"revision_id"`
	CanUndo       bool   `json:"can_undo"`
	CanRedo       bool   `json:"can_redo"`
	UndoOperation string `json:"undo_operation,omitempty"`
	RedoOperation string `json:"redo_operation,omitempty"`
}

type cueDraftOrderEntry struct {
	CueID      string `json:"cue_id"`
	OrderIndex int    `json:"order_index"`
}

type cueDraftHistoryDelta struct {
	Version           int                         `json:"version"`
	BeforeHash        string                      `json:"before_sha256"`
	AfterHash         string                      `json:"after_sha256"`
	BeforeCue         *domain.Cue                 `json:"before_cue,omitempty"`
	AfterCue          *domain.Cue                 `json:"after_cue,omitempty"`
	BeforeOrder       []cueDraftOrderEntry        `json:"before_order"`
	AfterOrder        []cueDraftOrderEntry        `json:"after_order"`
	BeforeCheckpoints []cueDraftHistoryCheckpoint `json:"before_checkpoints,omitempty"`
	AfterCheckpoints  []cueDraftHistoryCheckpoint `json:"after_checkpoints,omitempty"`
}

type cueDraftHistoryCheckpoint struct {
	Order     int64           `json:"order"`
	ID        string          `json:"checkpoint_id"`
	ProjectID string          `json:"project_id"`
	UserID    string          `json:"user_id"`
	CueID     string          `json:"cue_id"`
	Name      string          `json:"name"`
	CreatedAt string          `json:"created_at"`
	Document  json.RawMessage `json:"document"`
}

type cueDraftState struct {
	Cues        []domain.Cue
	Order       []cueDraftOrderEntry
	Checkpoints []cueDraftHistoryCheckpoint
	Hash        string
}

func readCueDraftStateTx(ctx context.Context, tx *sql.Tx, revisionID, checkpointCueID string) (cueDraftState, error) {
	state := cueDraftState{Cues: []domain.Cue{}, Order: []cueDraftOrderEntry{}, Checkpoints: []cueDraftHistoryCheckpoint{}}
	rows, err := tx.QueryContext(ctx, `
		SELECT cue_id, revision_id, display_label, name, order_index, cue_type,
		       criticality, enabled, execution_policy_json, notes_summary
		FROM cues WHERE revision_id = ? ORDER BY order_index, cue_id
	`, revisionID)
	if err != nil {
		return state, fmt.Errorf("read Draft Cues for history: %w", err)
	}
	indices := make(map[string]int)
	for rows.Next() {
		var cue domain.Cue
		var enabled int
		var policy string
		if err := rows.Scan(&cue.ID, &cue.RevisionID, &cue.DisplayLabel, &cue.Name, &cue.OrderIndex,
			&cue.CueType, &cue.Criticality, &enabled, &policy, &cue.NotesSummary); err != nil {
			_ = rows.Close()
			return state, fmt.Errorf("scan Draft Cue history state: %w", err)
		}
		cue.Enabled = enabled == 1
		cue.ExecutionPolicy = json.RawMessage(policy)
		cue.Actions = []domain.Action{}
		indices[cue.ID] = len(state.Cues)
		state.Order = append(state.Order, cueDraftOrderEntry{CueID: cue.ID, OrderIndex: cue.OrderIndex})
		state.Cues = append(state.Cues, cue)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return state, fmt.Errorf("iterate Draft Cue history state: %w", err)
	}
	if err := rows.Close(); err != nil {
		return state, fmt.Errorf("close Draft Cue history state: %w", err)
	}

	actions, err := tx.QueryContext(ctx, `
		SELECT a.action_id, a.cue_id, a.order_index, a.execution_mode, a.target_ref,
		       a.capability_key, a.parameters_json, a.timeout_policy_json,
		       a.error_policy_json, a.priority_class, a.enabled
		FROM actions a JOIN cues c ON c.cue_id = a.cue_id
		WHERE c.revision_id = ?
		ORDER BY c.order_index, c.cue_id, a.order_index, a.action_id
	`, revisionID)
	if err != nil {
		return state, fmt.Errorf("read Draft Actions for history: %w", err)
	}
	for actions.Next() {
		var action domain.Action
		var parameters, timeoutPolicy, errorPolicy, priority string
		var enabled int
		if err := actions.Scan(&action.ID, &action.CueID, &action.OrderIndex, &action.ExecutionMode,
			&action.TargetRef, &action.CapabilityKey, &parameters, &timeoutPolicy, &errorPolicy,
			&priority, &enabled); err != nil {
			_ = actions.Close()
			return state, fmt.Errorf("scan Draft Action history state: %w", err)
		}
		index, ok := indices[action.CueID]
		if !ok {
			_ = actions.Close()
			return state, fmt.Errorf("Draft Action %s has no Cue in history state", action.ID)
		}
		action.Parameters = json.RawMessage(parameters)
		action.TimeoutPolicy = json.RawMessage(timeoutPolicy)
		action.ErrorPolicy = json.RawMessage(errorPolicy)
		action.PriorityClass = domain.PriorityClass(priority)
		action.Enabled = enabled == 1
		state.Cues[index].Actions = append(state.Cues[index].Actions, action)
	}
	if err := actions.Err(); err != nil {
		_ = actions.Close()
		return state, fmt.Errorf("iterate Draft Action history state: %w", err)
	}
	if err := actions.Close(); err != nil {
		return state, fmt.Errorf("close Draft Action history state: %w", err)
	}
	if checkpointCueID != "" {
		checkpoints, err := readCueDraftCheckpointsTx(ctx, tx, checkpointCueID)
		if err != nil {
			return state, err
		}
		state.Checkpoints = checkpoints
	}
	encoded, err := json.Marshal(state.Cues)
	if err != nil {
		return state, fmt.Errorf("encode Draft Cue history state: %w", err)
	}
	digest := sha256.Sum256(encoded)
	state.Hash = hex.EncodeToString(digest[:])
	return state, nil
}

func readCueDraftCheckpointsTx(ctx context.Context, tx *sql.Tx, cueID string) ([]cueDraftHistoryCheckpoint, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT checkpoint_order, checkpoint_id, project_id, user_id, cue_id, name, created_at, document
		FROM cue_editor_checkpoints WHERE cue_id = ? ORDER BY checkpoint_order
	`, cueID)
	if err != nil {
		if cueEditorCheckpointSchemaMissing(err) {
			return []cueDraftHistoryCheckpoint{}, nil
		}
		return nil, fmt.Errorf("read Cue checkpoints for history: %w", err)
	}
	defer rows.Close()
	checkpoints := []cueDraftHistoryCheckpoint{}
	for rows.Next() {
		var entry cueDraftHistoryCheckpoint
		var document string
		if err := rows.Scan(&entry.Order, &entry.ID, &entry.ProjectID, &entry.UserID,
			&entry.CueID, &entry.Name, &entry.CreatedAt, &document); err != nil {
			return nil, fmt.Errorf("scan Cue checkpoint for history: %w", err)
		}
		entry.Document = json.RawMessage(document)
		checkpoints = append(checkpoints, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Cue checkpoints for history: %w", err)
	}
	return checkpoints, nil
}

func cueEditorCheckpointSchemaMissing(err error) bool {
	var sqliteErr *sqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code() == 1 &&
		strings.Contains(sqliteErr.Error(), "no such table: cue_editor_checkpoints")
}

func cueDraftStateCue(state cueDraftState, cueID string) *domain.Cue {
	for i := range state.Cues {
		if state.Cues[i].ID == cueID {
			copy := state.Cues[i]
			return &copy
		}
	}
	return nil
}

func cueDraftOrdersEqual(a, b []cueDraftOrderEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func recordCueDraftHistoryTx(ctx context.Context, tx *sql.Tx, revisionID, operation, cueID string, before, after cueDraftState, nowUS int64) error {
	if before.Hash == after.Hash {
		return nil
	}
	actor := cueDraftHistoryActor(ctx)
	if len(actor) > 256 {
		return fmt.Errorf("%w: Cue history actor is too long", domain.ErrInvalidInput)
	}
	delta := cueDraftHistoryDelta{
		Version: 1, BeforeHash: before.Hash, AfterHash: after.Hash,
		BeforeCue: cueDraftStateCue(before, cueID), AfterCue: cueDraftStateCue(after, cueID),
		BeforeOrder: before.Order, AfterOrder: after.Order,
	}
	if operation == "CREATE" || operation == "DELETE" {
		delta.BeforeCheckpoints = before.Checkpoints
		delta.AfterCheckpoints = after.Checkpoints
	}
	encoded, err := json.Marshal(delta)
	if err != nil {
		return fmt.Errorf("encode Cue Draft history delta: %w", err)
	}
	if len(encoded) > 4<<20 {
		return fmt.Errorf("%w: Cue Draft history entry exceeds 4 MiB", domain.ErrInvalidInput)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO cue_draft_history_state (revision_id, actor_id)
		VALUES (?, ?)
	`, revisionID, actor); err != nil {
		if cueDraftHistorySchemaMissing(err) {
			// Historical migration tests deliberately exercise Store operations
			// before migration 60 exists. Production startup always applies all
			// migrations before constructing handlers.
			return nil
		}
		return fmt.Errorf("initialize Cue Draft history: %w", err)
	}
	var nextSequence, cursor int64
	if err := tx.QueryRowContext(ctx, `
		SELECT next_sequence, cursor_sequence
		FROM cue_draft_history_state WHERE revision_id = ? AND actor_id = ?
	`, revisionID, actor).Scan(&nextSequence, &cursor); err != nil {
		return fmt.Errorf("read Cue Draft history cursor: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM cue_draft_history_events
		WHERE revision_id = ? AND actor_id = ? AND sequence > ?
	`, revisionID, actor, cursor); err != nil {
		return fmt.Errorf("discard Cue Draft redo history: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO cue_draft_history_events
		(revision_id, actor_id, sequence, operation, cue_id, created_at_us, delta_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, revisionID, actor, nextSequence, operation, cueID, nowUS, string(encoded)); err != nil {
		return fmt.Errorf("append Cue Draft history: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE cue_draft_history_state
		SET cursor_sequence = ?, next_sequence = ?
		WHERE revision_id = ? AND actor_id = ?
	`, nextSequence, nextSequence+1, revisionID, actor); err != nil {
		return fmt.Errorf("advance Cue Draft history cursor: %w", err)
	}
	oldest := nextSequence - cueDraftHistoryLimit
	if oldest > 0 {
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM cue_draft_history_events
			WHERE revision_id = ? AND actor_id = ? AND sequence <= ?
		`, revisionID, actor, oldest); err != nil {
			return fmt.Errorf("bound Cue Draft history: %w", err)
		}
	}
	return nil
}

func cueDraftHistorySchemaMissing(err error) bool {
	var sqliteErr *sqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code() == 1 &&
		strings.Contains(sqliteErr.Error(), "no such table: cue_draft_history_state")
}

func readCurrentProjectDraftTx(ctx context.Context, tx *sql.Tx, projectID string) (string, error) {
	var revisionID, status string
	err := tx.QueryRowContext(ctx, `
		SELECT r.revision_id, r.status
		FROM projects p JOIN project_revisions r ON r.revision_id = p.current_revision_id
		WHERE p.project_id = ? AND r.project_id = p.project_id
	`, projectID).Scan(&revisionID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read current Cue Draft revision: %w", err)
	}
	if domain.RevisionStatus(status) != domain.RevisionDraft {
		return "", domain.ErrRevisionFrozen
	}
	return revisionID, nil
}

func (s *Store) CueDraftHistory(ctx context.Context, projectID string) (CueDraftHistoryStatus, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return CueDraftHistoryStatus{}, fmt.Errorf("%w: Project is required", domain.ErrInvalidInput)
	}
	project, err := s.GetProject(ctx, projectID)
	if err != nil {
		return CueDraftHistoryStatus{}, err
	}
	status := CueDraftHistoryStatus{ProjectID: projectID, RevisionID: project.CurrentRevisionID}
	revision, err := s.GetRevision(ctx, project.CurrentRevisionID)
	if err != nil {
		return CueDraftHistoryStatus{}, err
	}
	if revision.Status != domain.RevisionDraft {
		return status, nil
	}
	actor := cueDraftHistoryActor(ctx)
	var cursor int64
	err = s.db.QueryRowContext(ctx, `
		SELECT cursor_sequence FROM cue_draft_history_state
		WHERE revision_id = ? AND actor_id = ?
	`, revision.ID, actor).Scan(&cursor)
	if errors.Is(err, sql.ErrNoRows) {
		return status, nil
	}
	if err != nil {
		return CueDraftHistoryStatus{}, fmt.Errorf("read Cue Draft history status: %w", err)
	}
	var operation string
	err = s.db.QueryRowContext(ctx, `
		SELECT operation FROM cue_draft_history_events
		WHERE revision_id = ? AND actor_id = ? AND sequence = ?
	`, revision.ID, actor, cursor).Scan(&operation)
	if err == nil {
		status.CanUndo = true
		status.UndoOperation = operation
	} else if !errors.Is(err, sql.ErrNoRows) {
		return CueDraftHistoryStatus{}, fmt.Errorf("read Cue Draft undo availability: %w", err)
	}
	operation = ""
	err = s.db.QueryRowContext(ctx, `
		SELECT operation FROM cue_draft_history_events
		WHERE revision_id = ? AND actor_id = ? AND sequence = ?
	`, revision.ID, actor, cursor+1).Scan(&operation)
	if err == nil {
		status.CanRedo = true
		status.RedoOperation = operation
	} else if !errors.Is(err, sql.ErrNoRows) {
		return CueDraftHistoryStatus{}, fmt.Errorf("read Cue Draft redo availability: %w", err)
	}
	return status, nil
}

func (s *Store) UndoCueDraft(ctx context.Context, projectID string) (CueDraftHistoryStatus, error) {
	return s.moveCueDraftHistory(ctx, projectID, false)
}

func (s *Store) RedoCueDraft(ctx context.Context, projectID string) (CueDraftHistoryStatus, error) {
	return s.moveCueDraftHistory(ctx, projectID, true)
}

func (s *Store) moveCueDraftHistory(ctx context.Context, projectID string, redo bool) (CueDraftHistoryStatus, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return CueDraftHistoryStatus{}, fmt.Errorf("%w: Project is required", domain.ErrInvalidInput)
	}
	if err := s.RequireProjectConfigurationMutable(ctx, projectID); err != nil {
		return CueDraftHistoryStatus{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CueDraftHistoryStatus{}, fmt.Errorf("begin Cue Draft history move: %w", err)
	}
	defer tx.Rollback()
	revisionID, err := readCurrentProjectDraftTx(ctx, tx, projectID)
	if err != nil {
		return CueDraftHistoryStatus{}, err
	}
	if err := s.ensureDraft(ctx, tx, revisionID); err != nil {
		return CueDraftHistoryStatus{}, err
	}
	actor := cueDraftHistoryActor(ctx)
	var cursor int64
	err = tx.QueryRowContext(ctx, `
		SELECT cursor_sequence FROM cue_draft_history_state
		WHERE revision_id = ? AND actor_id = ?
	`, revisionID, actor).Scan(&cursor)
	if errors.Is(err, sql.ErrNoRows) {
		return CueDraftHistoryStatus{}, domain.ErrNotFound
	}
	if err != nil {
		return CueDraftHistoryStatus{}, fmt.Errorf("read Cue Draft history cursor: %w", err)
	}
	sequence := cursor
	if redo {
		sequence++
	}
	var operation, cueID, rawDelta string
	err = tx.QueryRowContext(ctx, `
		SELECT operation, cue_id, delta_json
		FROM cue_draft_history_events
		WHERE revision_id = ? AND actor_id = ? AND sequence = ?
	`, revisionID, actor, sequence).Scan(&operation, &cueID, &rawDelta)
	if errors.Is(err, sql.ErrNoRows) {
		return CueDraftHistoryStatus{}, domain.ErrNotFound
	}
	if err != nil {
		return CueDraftHistoryStatus{}, fmt.Errorf("read Cue Draft history event: %w", err)
	}
	var delta cueDraftHistoryDelta
	if err := json.Unmarshal([]byte(rawDelta), &delta); err != nil || delta.Version != 1 {
		return CueDraftHistoryStatus{}, fmt.Errorf("%w: invalid Cue Draft history event", domain.ErrConflict)
	}
	current, err := readCueDraftStateTx(ctx, tx, revisionID, cueID)
	if err != nil {
		return CueDraftHistoryStatus{}, err
	}
	checkpointGuard := operation == "CREATE" || operation == "DELETE"
	if checkpointGuard {
		expectedCheckpoints := delta.AfterCheckpoints
		if redo {
			expectedCheckpoints = delta.BeforeCheckpoints
		}
		if !cueDraftHistoryCheckpointsEqual(current.Checkpoints, expectedCheckpoints) {
			return CueDraftHistoryStatus{}, fmt.Errorf("%w: this Cue's named checkpoints changed after the history entry", domain.ErrConflict)
		}
	}
	expectedHash := delta.AfterHash
	if redo {
		expectedHash = delta.BeforeHash
	}
	if current.Hash != expectedHash {
		return CueDraftHistoryStatus{}, fmt.Errorf("%w: Draft changed after this editor's Cue history entry", domain.ErrConflict)
	}
	if err := restoreCueDraftHistoryDeltaTx(ctx, tx, revisionID, cueID, delta, redo); err != nil {
		return CueDraftHistoryStatus{}, err
	}
	updated, err := readCueDraftStateTx(ctx, tx, revisionID, cueID)
	if err != nil {
		return CueDraftHistoryStatus{}, err
	}
	if checkpointGuard && !cueDraftHistoryCheckpointsEqual(updated.Checkpoints, targetCueDraftHistoryCheckpoints(delta, redo)) {
		return CueDraftHistoryStatus{}, fmt.Errorf("%w: restored named checkpoints do not match the history entry", domain.ErrConflict)
	}
	wantHash := delta.BeforeHash
	if redo {
		wantHash = delta.AfterHash
	}
	if updated.Hash != wantHash {
		return CueDraftHistoryStatus{}, fmt.Errorf("%w: restored Cue history does not match its recorded state", domain.ErrConflict)
	}
	newCursor := sequence
	if !redo {
		newCursor--
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE cue_draft_history_state SET cursor_sequence = ?
		WHERE revision_id = ? AND actor_id = ? AND cursor_sequence = ?
	`, newCursor, revisionID, actor, cursor)
	if err != nil {
		return CueDraftHistoryStatus{}, fmt.Errorf("advance Cue Draft history cursor: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return CueDraftHistoryStatus{}, fmt.Errorf("Cue Draft history cursor rows affected: %w", err)
		}
		return CueDraftHistoryStatus{}, domain.ErrConflict
	}
	if err := capability.CheckWriteAuthorization(ctx, tx); err != nil {
		return CueDraftHistoryStatus{}, err
	}
	if err := tx.Commit(); err != nil {
		return CueDraftHistoryStatus{}, fmt.Errorf("commit Cue Draft history move: %w", err)
	}
	return s.CueDraftHistory(ctx, projectID)
}

func restoreCueDraftHistoryDeltaTx(ctx context.Context, tx *sql.Tx, revisionID, cueID string, delta cueDraftHistoryDelta, redo bool) error {
	target := delta.BeforeCue
	order := delta.BeforeOrder
	if redo {
		target, order = delta.AfterCue, delta.AfterOrder
	}
	if target != nil && target.ID != cueID {
		return fmt.Errorf("%w: history Cue identity mismatch", domain.ErrConflict)
	}
	if target == nil && cueID == "" && !cueDraftOrderOnlyDelta(delta) {
		return fmt.Errorf("%w: missing Cue identity in history entry", domain.ErrConflict)
	}
	current, err := readCueDraftStateTx(ctx, tx, revisionID, cueID)
	if err != nil {
		return err
	}
	currentIDs := make(map[string]struct{}, len(current.Cues))
	for _, cue := range current.Cues {
		currentIDs[cue.ID] = struct{}{}
	}
	targetIDs := make(map[string]struct{}, len(order))
	seenIndexes := make(map[int]struct{}, len(order))
	for _, entry := range order {
		if entry.CueID == "" || entry.OrderIndex < 0 {
			return fmt.Errorf("%w: invalid Cue Draft order in history", domain.ErrConflict)
		}
		if _, duplicate := targetIDs[entry.CueID]; duplicate {
			return fmt.Errorf("%w: duplicate Cue in history order", domain.ErrConflict)
		}
		if _, duplicate := seenIndexes[entry.OrderIndex]; duplicate {
			return fmt.Errorf("%w: duplicate Cue position in history order", domain.ErrConflict)
		}
		targetIDs[entry.CueID] = struct{}{}
		seenIndexes[entry.OrderIndex] = struct{}{}
	}
	if target != nil {
		targetIDs[target.ID] = struct{}{}
	}
	if len(targetIDs) != len(order) {
		return fmt.Errorf("%w: history Cue order omits a Cue", domain.ErrConflict)
	}
	for id := range currentIDs {
		if _, exists := targetIDs[id]; !exists {
			if id == cueID && target == nil {
				continue
			}
			return fmt.Errorf("%w: unrelated Cue changed since history entry", domain.ErrConflict)
		}
	}
	for id := range targetIDs {
		if _, exists := currentIDs[id]; !exists && (target == nil || id != target.ID) {
			return fmt.Errorf("%w: history would create an unrelated Cue", domain.ErrConflict)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE cues SET order_index = order_index + ? WHERE revision_id = ?
	`, cueDraftHistoryTempOrder, revisionID); err != nil {
		return fmt.Errorf("stage Cue Draft order for history restore: %w", err)
	}
	if target != nil {
		if target.RevisionID != revisionID {
			return fmt.Errorf("%w: history Cue belongs to another revision", domain.ErrConflict)
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE cues SET display_label = ?, name = ?, cue_type = ?, criticality = ?,
				enabled = ?, execution_policy_json = ?, notes_summary = ?
			WHERE cue_id = ? AND revision_id = ?
		`, target.DisplayLabel, target.Name, target.CueType, target.Criticality, boolInt(target.Enabled),
			string(target.ExecutionPolicy), target.NotesSummary, target.ID, revisionID)
		if err != nil {
			return fmt.Errorf("update Cue for history restore: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("Cue history restore rows affected: %w", err)
		}
		cueRestored := affected == 0
		if cueRestored {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO cues (cue_id, revision_id, display_label, name, order_index, cue_type,
					criticality, enabled, execution_policy_json, notes_summary)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, target.ID, revisionID, target.DisplayLabel, target.Name, cueDraftHistoryTempOrder+len(current.Cues)+1,
				target.CueType, target.Criticality, boolInt(target.Enabled), string(target.ExecutionPolicy), target.NotesSummary); err != nil {
				return fmt.Errorf("restore deleted Cue: %w", err)
			}
		}
		if err := restoreCueActionsTx(ctx, tx, *target); err != nil {
			return err
		}
		if cueRestored {
			if err := restoreCueDraftHistoryCheckpointsTx(ctx, tx, targetCueDraftHistoryCheckpoints(delta, redo)); err != nil {
				return err
			}
		}
	} else if cueID != "" {
		var referenced int
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM route_actions WHERE cue_id = ?
				UNION ALL SELECT 1 FROM sessions WHERE start_cue_id = ? OR last_completed_cue_id = ? OR next_cue_id = ? OR current_cue_id = ?
				UNION ALL SELECT 1 FROM cue_executions WHERE cue_id = ?
				UNION ALL SELECT 1 FROM operator_notes WHERE cue_id = ?
				UNION ALL SELECT 1 FROM simulation_checkpoints WHERE current_cue_id = ? OR last_completed_cue_id = ? OR next_cue_id = ?
			)
		`, cueID, cueID, cueID, cueID, cueID, cueID, cueID, cueID, cueID, cueID).Scan(&referenced); err != nil {
			return fmt.Errorf("check Cue references before history restore: %w", err)
		}
		if referenced != 0 {
			return fmt.Errorf("%w: Cue is referenced by routes or rehearsal records", domain.ErrConflict)
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM cues WHERE cue_id = ? AND revision_id = ?`, cueID, revisionID)
		if err != nil {
			return fmt.Errorf("remove Cue for history restore: %w", err)
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			if err != nil {
				return fmt.Errorf("delete Cue history rows affected: %w", err)
			}
			return domain.ErrConflict
		}
	}
	for _, entry := range order {
		result, err := tx.ExecContext(ctx, `
			UPDATE cues SET order_index = ? WHERE cue_id = ? AND revision_id = ?
		`, entry.OrderIndex, entry.CueID, revisionID)
		if err != nil {
			return fmt.Errorf("restore Cue position: %w", err)
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			if err != nil {
				return fmt.Errorf("Cue order rows affected: %w", err)
			}
			return domain.ErrConflict
		}
	}
	return nil
}

func cueDraftOrderOnlyDelta(delta cueDraftHistoryDelta) bool {
	return delta.BeforeCue == nil && delta.AfterCue == nil
}

func restoreCueDraftHistoryCheckpointsTx(ctx context.Context, tx *sql.Tx, checkpoints []cueDraftHistoryCheckpoint) error {
	for _, checkpoint := range checkpoints {
		if checkpoint.Order <= 0 || checkpoint.ID == "" || checkpoint.ProjectID == "" || checkpoint.UserID == "" ||
			checkpoint.CueID == "" || checkpoint.Name == "" || !json.Valid(checkpoint.Document) {
			return fmt.Errorf("%w: invalid named Cue checkpoint in history", domain.ErrConflict)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO cue_editor_checkpoints
			(checkpoint_order, checkpoint_id, project_id, user_id, cue_id, name, created_at, document)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, checkpoint.Order, checkpoint.ID, checkpoint.ProjectID, checkpoint.UserID, checkpoint.CueID,
			checkpoint.Name, checkpoint.CreatedAt, string(checkpoint.Document)); err != nil {
			return fmt.Errorf("restore named Cue checkpoint from history: %w", err)
		}
	}
	return nil
}

func targetCueDraftHistoryCheckpoints(delta cueDraftHistoryDelta, redo bool) []cueDraftHistoryCheckpoint {
	if redo {
		return delta.AfterCheckpoints
	}
	return delta.BeforeCheckpoints
}

func cueDraftHistoryCheckpointsEqual(a, b []cueDraftHistoryCheckpoint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Order != b[i].Order || a[i].ID != b[i].ID || a[i].ProjectID != b[i].ProjectID ||
			a[i].UserID != b[i].UserID || a[i].CueID != b[i].CueID || a[i].Name != b[i].Name ||
			a[i].CreatedAt != b[i].CreatedAt || !bytes.Equal(a[i].Document, b[i].Document) {
			return false
		}
	}
	return true
}

func restoreCueActionsTx(ctx context.Context, tx *sql.Tx, cue domain.Cue) error {
	desired := make(map[string]domain.Action, len(cue.Actions))
	for _, action := range cue.Actions {
		if action.ID == "" || action.CueID != cue.ID {
			return fmt.Errorf("%w: invalid Action identity in Cue history", domain.ErrConflict)
		}
		if _, duplicate := desired[action.ID]; duplicate {
			return fmt.Errorf("%w: duplicate Action in Cue history", domain.ErrConflict)
		}
		desired[action.ID] = action
	}
	rows, err := tx.QueryContext(ctx, `SELECT action_id FROM actions WHERE cue_id = ?`, cue.ID)
	if err != nil {
		return fmt.Errorf("list Actions for history restore: %w", err)
	}
	var remove []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan Action for history restore: %w", err)
		}
		if _, exists := desired[id]; !exists {
			remove = append(remove, id)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate Actions for history restore: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close Actions for history restore: %w", err)
	}
	for _, id := range remove {
		if _, err := tx.ExecContext(ctx, `DELETE FROM actions WHERE action_id = ? AND cue_id = ?`, id, cue.ID); err != nil {
			return fmt.Errorf("remove Action for history restore: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE actions SET order_index = order_index + ? WHERE cue_id = ?`, cueDraftHistoryTempOrder, cue.ID); err != nil {
		return fmt.Errorf("stage Action order for history restore: %w", err)
	}
	for _, action := range cue.Actions {
		var existingCueID string
		err := tx.QueryRowContext(ctx, `SELECT cue_id FROM actions WHERE action_id = ?`, action.ID).Scan(&existingCueID)
		if err == nil && existingCueID != cue.ID {
			return fmt.Errorf("%w: Action identity is now owned by another Cue", domain.ErrConflict)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("verify Action identity for history restore: %w", err)
		}
		params, err := normalizeJSON(action.Parameters, "{}")
		if err != nil {
			return fmt.Errorf("history Action parameters: %w", err)
		}
		timeoutPolicy, err := normalizeJSON(action.TimeoutPolicy, "{}")
		if err != nil {
			return fmt.Errorf("history Action timeout policy: %w", err)
		}
		errorPolicy, err := normalizeJSON(action.ErrorPolicy, "{}")
		if err != nil {
			return fmt.Errorf("history Action error policy: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO actions (action_id, cue_id, order_index, execution_mode, target_ref,
				capability_key, parameters_json, timeout_policy_json, error_policy_json, priority_class, enabled)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(action_id) DO UPDATE SET cue_id = excluded.cue_id,
				order_index = excluded.order_index, execution_mode = excluded.execution_mode,
				target_ref = excluded.target_ref, capability_key = excluded.capability_key,
				parameters_json = excluded.parameters_json, timeout_policy_json = excluded.timeout_policy_json,
				error_policy_json = excluded.error_policy_json, priority_class = excluded.priority_class,
				enabled = excluded.enabled
		`, action.ID, cue.ID, action.OrderIndex, action.ExecutionMode, action.TargetRef, action.CapabilityKey,
			params, timeoutPolicy, errorPolicy, action.PriorityClass, boolInt(action.Enabled)); err != nil {
			return fmt.Errorf("restore Action from Cue history: %w", err)
		}
	}
	return nil
}

// History timestamps use the Store's injected clock so tests remain stable.
func cueDraftHistoryTimestamp(c clock.Clock) int64 {
	return clock.UnixMicros(c.Now().UTC())
}
