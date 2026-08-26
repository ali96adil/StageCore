package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
)

type Store struct {
	db    *sql.DB
	clock clock.Clock
}

func New(database *sql.DB, c clock.Clock) *Store {
	if c == nil {
		c = clock.Real{}
	}
	return &Store{db: database, clock: c}
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) ensureDraft(ctx context.Context, q queryer, revisionID string) error {
	var status string
	if err := q.QueryRowContext(ctx, `SELECT status FROM project_revisions WHERE revision_id = ?`, revisionID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("read revision status: %w", err)
	}
	if domain.RevisionStatus(status) != domain.RevisionDraft {
		return domain.ErrRevisionFrozen
	}
	return nil
}

func (s *Store) entityBelongsToRevision(ctx context.Context, table, idColumn, entityID, revisionID string) (bool, error) {
	return s.entityBelongsToRevisionTx(ctx, s.db, table, idColumn, entityID, revisionID)
}

func (s *Store) entityBelongsToRevisionTx(ctx context.Context, q queryer, table, idColumn, entityID, revisionID string) (bool, error) {
	allowed := map[string]string{
		"input_definitions":  "input_id",
		"output_definitions": "output_id",
		"cues":               "cue_id",
	}
	if allowed[table] != idColumn {
		return false, fmt.Errorf("unsupported ownership query")
	}
	query := fmt.Sprintf("SELECT 1 FROM %s WHERE %s = ? AND revision_id = ?", table, idColumn)
	var one int
	err := q.QueryRowContext(ctx, query, entityID, revisionID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check revision ownership: %w", err)
	}
	return one == 1, nil
}

func normalizeJSON(raw json.RawMessage, fallback string) (string, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 {
		value = []byte(fallback)
	}
	if !json.Valid(value) {
		return "", fmt.Errorf("%w: malformed JSON", domain.ErrInvalidInput)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, value); err != nil {
		return "", fmt.Errorf("compact JSON: %w", err)
	}
	return compact.String(), nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
