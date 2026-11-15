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
	outputDraftHistoryLimit       = 50
	outputDraftHistorySystemActor = "_stagecore_system_"
)

type outputDraftHistoryActorKey struct{}

// WithOutputDraftHistoryActor scopes Output Undo/Redo to the authenticated
// local editor. Store callers without an actor use an internal history scope.
func WithOutputDraftHistoryActor(ctx context.Context, actorID string) context.Context {
	return context.WithValue(ctx, outputDraftHistoryActorKey{}, strings.TrimSpace(actorID))
}

func outputDraftHistoryActor(ctx context.Context) string {
	actor, _ := ctx.Value(outputDraftHistoryActorKey{}).(string)
	if actor = strings.TrimSpace(actor); actor == "" {
		return outputDraftHistorySystemActor
	}
	return actor
}

// OutputDraftHistoryStatus contains availability and operation labels only.
// Output definitions remain available through the normal configuration API.
type OutputDraftHistoryStatus struct {
	ProjectID     string `json:"project_id"`
	RevisionID    string `json:"revision_id"`
	CanUndo       bool   `json:"can_undo"`
	CanRedo       bool   `json:"can_redo"`
	UndoOperation string `json:"undo_operation,omitempty"`
	RedoOperation string `json:"redo_operation,omitempty"`
}

type outputDraftHistoryDelta struct {
	Version    int                      `json:"version"`
	BeforeHash string                   `json:"before_sha256"`
	AfterHash  string                   `json:"after_sha256"`
	Before     *domain.OutputDefinition `json:"before_output,omitempty"`
	After      *domain.OutputDefinition `json:"after_output,omitempty"`
}

func readOutputTx(ctx context.Context, tx *sql.Tx, revisionID, outputID string) (*domain.OutputDefinition, error) {
	var output domain.OutputDefinition
	var schema, criticality string
	err := tx.QueryRowContext(ctx, `
		SELECT output_id, revision_id, name, target_ref, capability_key, value_schema_json, criticality
		FROM output_definitions WHERE output_id = ? AND revision_id = ?
	`, outputID, revisionID).Scan(&output.ID, &output.RevisionID, &output.Name, &output.TargetRef,
		&output.CapabilityKey, &schema, &criticality)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read Output for Draft history: %w", err)
	}
	output.ValueSchema = json.RawMessage(schema)
	output.Criticality = criticality
	return &output, nil
}

func outputDraftHistoryHash(output *domain.OutputDefinition) (string, error) {
	if output == nil {
		return "absent", nil
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("encode Output Draft history state: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func recordOutputDraftHistoryTx(ctx context.Context, tx *sql.Tx, revisionID, operation, outputID string, before, after *domain.OutputDefinition, nowUS int64) error {
	beforeHash, err := outputDraftHistoryHash(before)
	if err != nil {
		return err
	}
	afterHash, err := outputDraftHistoryHash(after)
	if err != nil {
		return err
	}
	if beforeHash == afterHash {
		return nil
	}
	actor := outputDraftHistoryActor(ctx)
	if len(actor) > 256 {
		return fmt.Errorf("%w: Output history actor is too long", domain.ErrInvalidInput)
	}
	encoded, err := json.Marshal(outputDraftHistoryDelta{
		Version: 1, BeforeHash: beforeHash, AfterHash: afterHash, Before: before, After: after,
	})
	if err != nil {
		return fmt.Errorf("encode Output Draft history delta: %w", err)
	}
	if len(encoded) > 4<<20 {
		return fmt.Errorf("%w: Output Draft history entry exceeds 4 MiB", domain.ErrInvalidInput)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO output_draft_history_state (revision_id, actor_id) VALUES (?, ?)
	`, revisionID, actor); err != nil {
		if outputDraftHistorySchemaMissing(err) {
			// Migration tests deliberately exercise old schemas. Product startup
			// applies all migrations before handlers are registered.
			return nil
		}
		return fmt.Errorf("initialize Output Draft history: %w", err)
	}
	var nextSequence, cursor int64
	if err := tx.QueryRowContext(ctx, `
		SELECT next_sequence, cursor_sequence FROM output_draft_history_state
		WHERE revision_id = ? AND actor_id = ?
	`, revisionID, actor).Scan(&nextSequence, &cursor); err != nil {
		return fmt.Errorf("read Output Draft history cursor: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM output_draft_history_events
		WHERE revision_id = ? AND actor_id = ? AND sequence > ?
	`, revisionID, actor, cursor); err != nil {
		return fmt.Errorf("discard Output Draft redo history: %w", err)
	}
	// Reuse the first discarded sequence so a branched history has no cursor gap.
	nextSequence = cursor + 1
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO output_draft_history_events
		(revision_id, actor_id, sequence, operation, output_id, created_at_us, delta_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, revisionID, actor, nextSequence, operation, outputID, nowUS, string(encoded)); err != nil {
		return fmt.Errorf("append Output Draft history: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE output_draft_history_state SET cursor_sequence = ?, next_sequence = ?
		WHERE revision_id = ? AND actor_id = ?
	`, nextSequence, nextSequence+1, revisionID, actor); err != nil {
		return fmt.Errorf("advance Output Draft history cursor: %w", err)
	}
	oldest := nextSequence - outputDraftHistoryLimit
	if oldest > 0 {
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM output_draft_history_events
			WHERE revision_id = ? AND actor_id = ? AND sequence <= ?
		`, revisionID, actor, oldest); err != nil {
			return fmt.Errorf("bound Output Draft history: %w", err)
		}
	}
	return nil
}

func outputDraftHistorySchemaMissing(err error) bool {
	var sqliteErr *sqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code() == 1 &&
		strings.Contains(sqliteErr.Error(), "no such table: output_draft_history_state")
}

func (s *Store) OutputDraftHistory(ctx context.Context, projectID string) (OutputDraftHistoryStatus, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return OutputDraftHistoryStatus{}, fmt.Errorf("%w: Project is required", domain.ErrInvalidInput)
	}
	project, err := s.GetProject(ctx, projectID)
	if err != nil {
		return OutputDraftHistoryStatus{}, err
	}
	status := OutputDraftHistoryStatus{ProjectID: projectID, RevisionID: project.CurrentRevisionID}
	revision, err := s.GetRevision(ctx, project.CurrentRevisionID)
	if err != nil {
		return OutputDraftHistoryStatus{}, err
	}
	if revision.Status != domain.RevisionDraft {
		return status, nil
	}
	actor := outputDraftHistoryActor(ctx)
	var cursor int64
	err = s.db.QueryRowContext(ctx, `
		SELECT cursor_sequence FROM output_draft_history_state WHERE revision_id = ? AND actor_id = ?
	`, revision.ID, actor).Scan(&cursor)
	if errors.Is(err, sql.ErrNoRows) {
		return status, nil
	}
	if err != nil {
		return OutputDraftHistoryStatus{}, fmt.Errorf("read Output Draft history status: %w", err)
	}
	var operation string
	err = s.db.QueryRowContext(ctx, `
		SELECT operation FROM output_draft_history_events
		WHERE revision_id = ? AND actor_id = ? AND sequence = ?
	`, revision.ID, actor, cursor).Scan(&operation)
	if err == nil {
		status.CanUndo, status.UndoOperation = true, operation
	} else if !errors.Is(err, sql.ErrNoRows) {
		return OutputDraftHistoryStatus{}, fmt.Errorf("read Output Draft undo availability: %w", err)
	}
	err = s.db.QueryRowContext(ctx, `
		SELECT operation FROM output_draft_history_events
		WHERE revision_id = ? AND actor_id = ? AND sequence = ?
	`, revision.ID, actor, cursor+1).Scan(&operation)
	if err == nil {
		status.CanRedo, status.RedoOperation = true, operation
	} else if !errors.Is(err, sql.ErrNoRows) {
		return OutputDraftHistoryStatus{}, fmt.Errorf("read Output Draft redo availability: %w", err)
	}
	return status, nil
}

func (s *Store) UndoOutputDraft(ctx context.Context, projectID string) (OutputDraftHistoryStatus, error) {
	return s.moveOutputDraftHistory(ctx, projectID, false)
}

func (s *Store) RedoOutputDraft(ctx context.Context, projectID string) (OutputDraftHistoryStatus, error) {
	return s.moveOutputDraftHistory(ctx, projectID, true)
}

func (s *Store) moveOutputDraftHistory(ctx context.Context, projectID string, redo bool) (OutputDraftHistoryStatus, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return OutputDraftHistoryStatus{}, fmt.Errorf("%w: Project is required", domain.ErrInvalidInput)
	}
	if err := s.RequireProjectConfigurationMutable(ctx, projectID); err != nil {
		return OutputDraftHistoryStatus{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return OutputDraftHistoryStatus{}, fmt.Errorf("begin Output Draft history move: %w", err)
	}
	defer tx.Rollback()
	revisionID, err := readCurrentProjectDraftTx(ctx, tx, projectID)
	if err != nil {
		return OutputDraftHistoryStatus{}, err
	}
	if err := s.ensureDraft(ctx, tx, revisionID); err != nil {
		return OutputDraftHistoryStatus{}, err
	}
	actor := outputDraftHistoryActor(ctx)
	var cursor int64
	err = tx.QueryRowContext(ctx, `
		SELECT cursor_sequence FROM output_draft_history_state WHERE revision_id = ? AND actor_id = ?
	`, revisionID, actor).Scan(&cursor)
	if errors.Is(err, sql.ErrNoRows) {
		return OutputDraftHistoryStatus{}, domain.ErrNotFound
	}
	if err != nil {
		return OutputDraftHistoryStatus{}, fmt.Errorf("read Output Draft history cursor: %w", err)
	}
	sequence := cursor
	if redo {
		sequence++
	}
	var operation, outputID, rawDelta string
	err = tx.QueryRowContext(ctx, `
		SELECT operation, output_id, delta_json FROM output_draft_history_events
		WHERE revision_id = ? AND actor_id = ? AND sequence = ?
	`, revisionID, actor, sequence).Scan(&operation, &outputID, &rawDelta)
	if errors.Is(err, sql.ErrNoRows) {
		return OutputDraftHistoryStatus{}, domain.ErrNotFound
	}
	if err != nil {
		return OutputDraftHistoryStatus{}, fmt.Errorf("read Output Draft history event: %w", err)
	}
	var delta outputDraftHistoryDelta
	if err := json.Unmarshal([]byte(rawDelta), &delta); err != nil || delta.Version != 1 {
		return OutputDraftHistoryStatus{}, fmt.Errorf("%w: invalid Output Draft history event", domain.ErrConflict)
	}
	current, err := readOutputTx(ctx, tx, revisionID, outputID)
	if err != nil {
		return OutputDraftHistoryStatus{}, err
	}
	currentHash, err := outputDraftHistoryHash(current)
	if err != nil {
		return OutputDraftHistoryStatus{}, err
	}
	expectedHash, target := delta.AfterHash, delta.Before
	if redo {
		expectedHash, target = delta.BeforeHash, delta.After
	}
	if currentHash != expectedHash {
		return OutputDraftHistoryStatus{}, fmt.Errorf("%w: Output changed after this editor's history entry", domain.ErrConflict)
	}
	if err := s.restoreOutputDraftHistoryTx(ctx, tx, revisionID, outputID, target); err != nil {
		return OutputDraftHistoryStatus{}, err
	}
	updated, err := readOutputTx(ctx, tx, revisionID, outputID)
	if err != nil {
		return OutputDraftHistoryStatus{}, err
	}
	wantHash := delta.BeforeHash
	if redo {
		wantHash = delta.AfterHash
	}
	updatedHash, err := outputDraftHistoryHash(updated)
	if err != nil {
		return OutputDraftHistoryStatus{}, err
	}
	if updatedHash != wantHash {
		return OutputDraftHistoryStatus{}, fmt.Errorf("%w: restored Output does not match its recorded state", domain.ErrConflict)
	}
	newCursor := sequence
	if !redo {
		newCursor--
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE output_draft_history_state SET cursor_sequence = ?
		WHERE revision_id = ? AND actor_id = ? AND cursor_sequence = ?
	`, newCursor, revisionID, actor, cursor)
	if err != nil {
		return OutputDraftHistoryStatus{}, fmt.Errorf("advance Output Draft history cursor: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return OutputDraftHistoryStatus{}, fmt.Errorf("Output Draft history cursor rows affected: %w", err)
		}
		return OutputDraftHistoryStatus{}, domain.ErrConflict
	}
	if err := capability.CheckWriteAuthorization(ctx, tx); err != nil {
		return OutputDraftHistoryStatus{}, err
	}
	if err := tx.Commit(); err != nil {
		return OutputDraftHistoryStatus{}, fmt.Errorf("commit Output Draft history move: %w", err)
	}
	return s.OutputDraftHistory(ctx, projectID)
}

func (s *Store) restoreOutputDraftHistoryTx(ctx context.Context, tx *sql.Tx, revisionID, outputID string, target *domain.OutputDefinition) error {
	current, err := readOutputTx(ctx, tx, revisionID, outputID)
	if err != nil {
		return err
	}
	if target == nil {
		if current == nil {
			return domain.ErrConflict
		}
		var references int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM route_actions WHERE output_id = ?`, outputID).Scan(&references); err != nil {
			return fmt.Errorf("count Output route references for history: %w", err)
		}
		if references > 0 {
			return fmt.Errorf("%w: Output is referenced by %d route(s)", domain.ErrConflict, references)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM output_definitions WHERE output_id = ? AND revision_id = ?`, outputID, revisionID); err != nil {
			return fmt.Errorf("remove Output from Draft history: %w", err)
		}
		return nil
	}
	if target.ID != outputID || target.RevisionID != revisionID {
		return fmt.Errorf("%w: Output history identity mismatch", domain.ErrConflict)
	}
	schema, err := normalizeJSON(target.ValueSchema, "{}")
	if err != nil {
		return fmt.Errorf("%w: invalid Output schema in history entry", domain.ErrConflict)
	}
	criticality := strings.TrimSpace(target.Criticality)
	if criticality == "" {
		criticality = "NORMAL"
	}
	if current == nil {
		var existingRevision string
		err := tx.QueryRowContext(ctx, `SELECT revision_id FROM output_definitions WHERE output_id = ?`, outputID).Scan(&existingRevision)
		if err == nil {
			return fmt.Errorf("%w: Output identity was reused by another revision", domain.ErrConflict)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read Output identity before history restore: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO output_definitions (output_id, revision_id, name, target_ref, capability_key, value_schema_json, criticality)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, outputID, revisionID, strings.TrimSpace(target.Name), target.TargetRef, target.CapabilityKey, schema, criticality); err != nil {
			return fmt.Errorf("recreate Output from history: %w", err)
		}
		return nil
	}
	if current.RevisionID != revisionID {
		return fmt.Errorf("%w: Output belongs to another revision", domain.ErrConflict)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE output_definitions SET name = ?, target_ref = ?, capability_key = ?, value_schema_json = ?, criticality = ?
		WHERE output_id = ? AND revision_id = ?
	`, strings.TrimSpace(target.Name), target.TargetRef, target.CapabilityKey, schema,
		criticality, outputID, revisionID); err != nil {
		return fmt.Errorf("restore Output history: %w", err)
	}
	return nil
}
