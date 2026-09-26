package store

import (
	"context"
	"fmt"

	"github.com/ali96adil/StageCore/internal/domain"
)

func (s *Store) ListCompanions(ctx context.Context) ([]domain.Companion, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT companion_id, display_name, hostname, platform, architecture, version,
		       capabilities_json, last_seen_at_us, trust_state, readiness,
		       applied_runtime_snapshot_id, config_hash, created_at_us, updated_at_us
		FROM companions
		ORDER BY display_name, companion_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list companions: %w", err)
	}
	defer rows.Close()

	items := make([]domain.Companion, 0)
	for rows.Next() {
		companion, err := scanCompanion(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, companion)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate companions: %w", err)
	}
	return items, nil
}
