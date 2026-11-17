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
	inputDraftHistoryLimit       = 50
	inputDraftHistorySystemActor = "_stagecore_system_"
)

type inputDraftHistoryActorKey struct{}

// WithInputDraftHistoryActor scopes Input Undo/Redo to the authenticated
// local editor. Store callers without an actor use an internal history scope.
func WithInputDraftHistoryActor(ctx context.Context, actorID string) context.Context {
	return context.WithValue(ctx, inputDraftHistoryActorKey{}, strings.TrimSpace(actorID))
}

func inputDraftHistoryActor(ctx context.Context) string {
	actor, _ := ctx.Value(inputDraftHistoryActorKey{}).(string)
	if actor = strings.TrimSpace(actor); actor == "" {
		return inputDraftHistorySystemActor
	}
	return actor
}

// InputDraftHistoryStatus contains availability and operation labels only.
// Input definitions remain available through the normal configuration API.
type InputDraftHistoryStatus struct {
	ProjectID     string `json:"project_id"`
	RevisionID    string `json:"revision_id"`
	CanUndo       bool   `json:"can_undo"`
	CanRedo       bool   `json:"can_redo"`
	UndoOperation string `json:"undo_operation,omitempty"`
	RedoOperation string `json:"redo_operation,omitempty"`
}

type inputDraftHistoryDelta struct {
	Version    int                     `json:"version"`
	BeforeHash string                  `json:"before_sha256"`
	AfterHash  string                  `json:"after_sha256"`
	Before     *domain.InputDefinition `json:"before_input,omitempty"`
	After      *domain.InputDefinition `json:"after_input,omitempty"`
}

func readInputTx(ctx context.Context, tx *sql.Tx, revisionID, inputID string) (*domain.InputDefinition, error) {
	var input domain.InputDefinition
	var schema string
	var enabled int
	err := tx.QueryRowContext(ctx, `
		SELECT input_id, revision_id, name, source_ref, event_type, value_schema_json, enabled
		FROM input_definitions WHERE input_id = ? AND revision_id = ?
	`, inputID, revisionID).Scan(&input.ID, &input.RevisionID, &input.Name, &input.SourceRef,
		&input.EventType, &schema, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read Input for Draft history: %w", err)
	}
	input.ValueSchema = json.RawMessage(schema)
	input.Enabled = enabled == 1
	return &input, nil
}

func inputDraftHistoryHash(input *domain.InputDefinition) (string, error) {
	if input == nil {
		return "absent", nil
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode Input Draft history state: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func recordInputDraftHistoryTx(ctx context.Context, tx *sql.Tx, revisionID, operation, inputID string, before, after *domain.InputDefinition, nowUS int64) error {
	beforeHash, err := inputDraftHistoryHash(before)
	if err != nil {
		return err
	}
	afterHash, err := inputDraftHistoryHash(after)
	if err != nil {
		return err
	}
	if beforeHash == afterHash {
		return nil
	}
	actor := inputDraftHistoryActor(ctx)
	if len(actor) > 256 {
		return fmt.Errorf("%w: Input history actor is too long", domain.ErrInvalidInput)
	}
	encoded, err := json.Marshal(inputDraftHistoryDelta{
		Version: 1, BeforeHash: beforeHash, AfterHash: afterHash, Before: before, After: after,
	})
	if err != nil {
		return fmt.Errorf("encode Input Draft history delta: %w", err)
	}
	if len(encoded) > 4<<20 {
		return fmt.Errorf("%w: Input Draft history entry exceeds 4 MiB", domain.ErrInvalidInput)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO input_draft_history_state (revision_id, actor_id) VALUES (?, ?)
	`, revisionID, actor); err != nil {
		if inputDraftHistorySchemaMissing(err) {
			// Migration tests deliberately exercise old schemas. Product startup
			// applies all migrations before handlers are registered.
			return nil
		}
		return fmt.Errorf("initialize Input Draft history: %w", err)
	}
	var nextSequence, cursor int64
	if err := tx.QueryRowContext(ctx, `
		SELECT next_sequence, cursor_sequence FROM input_draft_history_state
		WHERE revision_id = ? AND actor_id = ?
	`, revisionID, actor).Scan(&nextSequence, &cursor); err != nil {
		return fmt.Errorf("read Input Draft history cursor: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM input_draft_history_events
		WHERE revision_id = ? AND actor_id = ? AND sequence > ?
	`, revisionID, actor, cursor); err != nil {
		return fmt.Errorf("discard Input Draft redo history: %w", err)
	}
	// Reuse the first discarded sequence so a branched history has no cursor gap.
	nextSequence = cursor + 1
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO input_draft_history_events
		(revision_id, actor_id, sequence, operation, input_id, created_at_us, delta_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, revisionID, actor, nextSequence, operation, inputID, nowUS, string(encoded)); err != nil {
		return fmt.Errorf("append Input Draft history: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE input_draft_history_state SET cursor_sequence = ?, next_sequence = ?
		WHERE revision_id = ? AND actor_id = ?
	`, nextSequence, nextSequence+1, revisionID, actor); err != nil {
		return fmt.Errorf("advance Input Draft history cursor: %w", err)
	}
	oldest := nextSequence - inputDraftHistoryLimit
	if oldest > 0 {
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM input_draft_history_events
			WHERE revision_id = ? AND actor_id = ? AND sequence <= ?
		`, revisionID, actor, oldest); err != nil {
			return fmt.Errorf("bound Input Draft history: %w", err)
		}
	}
	return nil
}

func inputDraftHistorySchemaMissing(err error) bool {
	var sqliteErr *sqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code() == 1 &&
		strings.Contains(sqliteErr.Error(), "no such table: input_draft_history_state")
}

func (s *Store) InputDraftHistory(ctx context.Context, projectID string) (InputDraftHistoryStatus, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return InputDraftHistoryStatus{}, fmt.Errorf("%w: Project is required", domain.ErrInvalidInput)
	}
	project, err := s.GetProject(ctx, projectID)
	if err != nil {
		return InputDraftHistoryStatus{}, err
	}
	status := InputDraftHistoryStatus{ProjectID: projectID, RevisionID: project.CurrentRevisionID}
	revision, err := s.GetRevision(ctx, project.CurrentRevisionID)
	if err != nil {
		return InputDraftHistoryStatus{}, err
	}
	if revision.Status != domain.RevisionDraft {
		return status, nil
	}
	actor := inputDraftHistoryActor(ctx)
	var cursor int64
	err = s.db.QueryRowContext(ctx, `
		SELECT cursor_sequence FROM input_draft_history_state WHERE revision_id = ? AND actor_id = ?
	`, revision.ID, actor).Scan(&cursor)
	if errors.Is(err, sql.ErrNoRows) {
		return status, nil
	}
	if err != nil {
		return InputDraftHistoryStatus{}, fmt.Errorf("read Input Draft history status: %w", err)
	}
	var operation string
	err = s.db.QueryRowContext(ctx, `
		SELECT operation FROM input_draft_history_events
		WHERE revision_id = ? AND actor_id = ? AND sequence = ?
	`, revision.ID, actor, cursor).Scan(&operation)
	if err == nil {
		status.CanUndo, status.UndoOperation = true, operation
	} else if !errors.Is(err, sql.ErrNoRows) {
		return InputDraftHistoryStatus{}, fmt.Errorf("read Input Draft undo availability: %w", err)
	}
	err = s.db.QueryRowContext(ctx, `
		SELECT operation FROM input_draft_history_events
		WHERE revision_id = ? AND actor_id = ? AND sequence = ?
	`, revision.ID, actor, cursor+1).Scan(&operation)
	if err == nil {
		status.CanRedo, status.RedoOperation = true, operation
	} else if !errors.Is(err, sql.ErrNoRows) {
		return InputDraftHistoryStatus{}, fmt.Errorf("read Input Draft redo availability: %w", err)
	}
	return status, nil
}

func (s *Store) UndoInputDraft(ctx context.Context, projectID string) (InputDraftHistoryStatus, error) {
	return s.moveInputDraftHistory(ctx, projectID, false)
}

func (s *Store) RedoInputDraft(ctx context.Context, projectID string) (InputDraftHistoryStatus, error) {
	return s.moveInputDraftHistory(ctx, projectID, true)
}

func (s *Store) moveInputDraftHistory(ctx context.Context, projectID string, redo bool) (InputDraftHistoryStatus, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return InputDraftHistoryStatus{}, fmt.Errorf("%w: Project is required", domain.ErrInvalidInput)
	}
	if err := s.RequireProjectConfigurationMutable(ctx, projectID); err != nil {
		return InputDraftHistoryStatus{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return InputDraftHistoryStatus{}, fmt.Errorf("begin Input Draft history move: %w", err)
	}
	defer tx.Rollback()
	revisionID, err := readCurrentProjectDraftTx(ctx, tx, projectID)
	if err != nil {
		return InputDraftHistoryStatus{}, err
	}
	if err := s.ensureDraft(ctx, tx, revisionID); err != nil {
		return InputDraftHistoryStatus{}, err
	}
	actor := inputDraftHistoryActor(ctx)
	var cursor int64
	err = tx.QueryRowContext(ctx, `
		SELECT cursor_sequence FROM input_draft_history_state WHERE revision_id = ? AND actor_id = ?
	`, revisionID, actor).Scan(&cursor)
	if errors.Is(err, sql.ErrNoRows) {
		return InputDraftHistoryStatus{}, domain.ErrNotFound
	}
	if err != nil {
		return InputDraftHistoryStatus{}, fmt.Errorf("read Input Draft history cursor: %w", err)
	}
	sequence := cursor
	if redo {
		sequence++
	}
	var operation, inputID, rawDelta string
	err = tx.QueryRowContext(ctx, `
		SELECT operation, input_id, delta_json FROM input_draft_history_events
		WHERE revision_id = ? AND actor_id = ? AND sequence = ?
	`, revisionID, actor, sequence).Scan(&operation, &inputID, &rawDelta)
	if errors.Is(err, sql.ErrNoRows) {
		return InputDraftHistoryStatus{}, domain.ErrNotFound
	}
	if err != nil {
		return InputDraftHistoryStatus{}, fmt.Errorf("read Input Draft history event: %w", err)
	}
	var delta inputDraftHistoryDelta
	if err := json.Unmarshal([]byte(rawDelta), &delta); err != nil || delta.Version != 1 {
		return InputDraftHistoryStatus{}, fmt.Errorf("%w: invalid Input Draft history event", domain.ErrConflict)
	}
	current, err := readInputTx(ctx, tx, revisionID, inputID)
	if err != nil {
		return InputDraftHistoryStatus{}, err
	}
	currentHash, err := inputDraftHistoryHash(current)
	if err != nil {
		return InputDraftHistoryStatus{}, err
	}
	expectedHash, target := delta.AfterHash, delta.Before
	if redo {
		expectedHash, target = delta.BeforeHash, delta.After
	}
	if currentHash != expectedHash {
		return InputDraftHistoryStatus{}, fmt.Errorf("%w: Input changed after this editor's history entry", domain.ErrConflict)
	}
	if err := s.restoreInputDraftHistoryTx(ctx, tx, revisionID, inputID, target); err != nil {
		return InputDraftHistoryStatus{}, err
	}
	updated, err := readInputTx(ctx, tx, revisionID, inputID)
	if err != nil {
		return InputDraftHistoryStatus{}, err
	}
	wantHash := delta.BeforeHash
	if redo {
		wantHash = delta.AfterHash
	}
	updatedHash, err := inputDraftHistoryHash(updated)
	if err != nil {
		return InputDraftHistoryStatus{}, err
	}
	if updatedHash != wantHash {
		return InputDraftHistoryStatus{}, fmt.Errorf("%w: restored Input does not match its recorded state", domain.ErrConflict)
	}
	newCursor := sequence
	if !redo {
		newCursor--
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE input_draft_history_state SET cursor_sequence = ?
		WHERE revision_id = ? AND actor_id = ? AND cursor_sequence = ?
	`, newCursor, revisionID, actor, cursor)
	if err != nil {
		return InputDraftHistoryStatus{}, fmt.Errorf("advance Input Draft history cursor: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return InputDraftHistoryStatus{}, fmt.Errorf("Input Draft history cursor rows affected: %w", err)
		}
		return InputDraftHistoryStatus{}, domain.ErrConflict
	}
	if err := capability.CheckWriteAuthorization(ctx, tx); err != nil {
		return InputDraftHistoryStatus{}, err
	}
	if err := tx.Commit(); err != nil {
		return InputDraftHistoryStatus{}, fmt.Errorf("commit Input Draft history move: %w", err)
	}
	return s.InputDraftHistory(ctx, projectID)
}

func (s *Store) restoreInputDraftHistoryTx(ctx context.Context, tx *sql.Tx, revisionID, inputID string, target *domain.InputDefinition) error {
	current, err := readInputTx(ctx, tx, revisionID, inputID)
	if err != nil {
		return err
	}
	if target == nil {
		if current == nil {
			return domain.ErrConflict
		}
		var references int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM routes WHERE input_id = ?`, inputID).Scan(&references); err != nil {
			return fmt.Errorf("count Input route references for history: %w", err)
		}
		if references > 0 {
			return fmt.Errorf("%w: Input is referenced by %d route(s)", domain.ErrConflict, references)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM input_definitions WHERE input_id = ? AND revision_id = ?`, inputID, revisionID); err != nil {
			return fmt.Errorf("remove Input from Draft history: %w", err)
		}
		return nil
	}
	if target.ID != inputID || target.RevisionID != revisionID {
		return fmt.Errorf("%w: Input history identity mismatch", domain.ErrConflict)
	}
	schema, err := normalizeJSON(target.ValueSchema, "{}")
	if err != nil {
		return fmt.Errorf("%w: invalid Input schema in history entry", domain.ErrConflict)
	}
	if current == nil {
		var existingRevision string
		err := tx.QueryRowContext(ctx, `SELECT revision_id FROM input_definitions WHERE input_id = ?`, inputID).Scan(&existingRevision)
		if err == nil {
			return fmt.Errorf("%w: Input identity was reused by another revision", domain.ErrConflict)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read Input identity before history restore: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO input_definitions (input_id, revision_id, name, source_ref, event_type, value_schema_json, enabled)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, inputID, revisionID, strings.TrimSpace(target.Name), strings.TrimSpace(target.SourceRef), strings.TrimSpace(target.EventType), schema, boolInt(target.Enabled)); err != nil {
			return fmt.Errorf("recreate Input from history: %w", err)
		}
		return nil
	}
	if current.RevisionID != revisionID {
		return fmt.Errorf("%w: Input belongs to another revision", domain.ErrConflict)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE input_definitions SET name = ?, source_ref = ?, event_type = ?, value_schema_json = ?, enabled = ?
		WHERE input_id = ? AND revision_id = ?
	`, strings.TrimSpace(target.Name), strings.TrimSpace(target.SourceRef), strings.TrimSpace(target.EventType), schema,
		boolInt(target.Enabled), inputID, revisionID); err != nil {
		return fmt.Errorf("restore Input history: %w", err)
	}
	return nil
}
