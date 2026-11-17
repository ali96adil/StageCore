package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
)

func (s *Store) ListCompanions(ctx context.Context) ([]domain.Companion, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT companion_id, display_name, hostname, platform, architecture, version,
		       capabilities_json, midi_destinations_json, last_seen_at_us, trust_state, readiness,
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

// ListProjectMachineRoleCompanions returns unassigned Companions and those
// already assigned within the requested Project. Foreign Project assignments
// are omitted so a scoped role editor cannot enumerate another show's devices.
func (s *Store) ListProjectMachineRoleCompanions(ctx context.Context, projectID string) ([]domain.Companion, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, fmt.Errorf("%w: project id is required", domain.ErrInvalidInput)
	}
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.companion_id, c.display_name, c.hostname, c.platform, c.architecture, c.version,
		       c.capabilities_json, c.midi_destinations_json, c.last_seen_at_us, c.trust_state, c.readiness,
		       c.applied_runtime_snapshot_id, c.config_hash, c.created_at_us, c.updated_at_us
		FROM companions c
		WHERE NOT EXISTS (
			SELECT 1
			FROM role_assignments ra
			JOIN machine_roles mr ON mr.machine_role_id = ra.machine_role_id
			WHERE ra.companion_id = c.companion_id AND ra.state <> 'RELEASED' AND mr.project_id <> ?
		)
		ORDER BY c.display_name, c.companion_id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list Project machine-role Companions: %w", err)
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
		return nil, fmt.Errorf("iterate Project machine-role Companions: %w", err)
	}
	return items, nil
}
