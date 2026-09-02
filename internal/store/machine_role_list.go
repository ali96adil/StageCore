package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
)

func (s *Store) ListMachineRoles(ctx context.Context, projectID string) ([]domain.MachineRole, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, fmt.Errorf("%w: project id is required", domain.ErrInvalidInput)
	}
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT machine_role_id, project_id, role_key, display_name, required_capabilities_json,
		       required_runtime_snapshot_id, required_config_hash, required, created_at_us, updated_at_us
		FROM machine_roles
		WHERE project_id = ?
		ORDER BY role_key, machine_role_id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list machine roles: %w", err)
	}
	defer rows.Close()

	roles := make([]domain.MachineRole, 0)
	for rows.Next() {
		role, err := scanMachineRole(rows)
		if err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate machine roles: %w", err)
	}
	return roles, nil
}
