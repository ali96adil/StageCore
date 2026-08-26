package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

func (s *Store) CreateAlias(ctx context.Context, alias domain.ProjectDeviceAlias) (domain.ProjectDeviceAlias, error) {
	if alias.ID == "" {
		var err error
		alias.ID, err = stageid.New()
		if err != nil {
			return domain.ProjectDeviceAlias{}, err
		}
	}
	cfg, err := normalizeJSON(alias.ProjectConfig, "{}")
	if err != nil {
		return domain.ProjectDeviceAlias{}, fmt.Errorf("alias project config: %w", err)
	}
	if alias.LogicalType == "" {
		alias.LogicalType = "GENERIC"
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO project_device_aliases (alias_id, project_id, logical_name, logical_type, target_ref, group_name, project_config_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, alias.ID, alias.ProjectID, strings.TrimSpace(alias.LogicalName), alias.LogicalType, alias.TargetRef, alias.GroupName, cfg)
	if err != nil {
		return domain.ProjectDeviceAlias{}, fmt.Errorf("insert alias: %w", err)
	}
	alias.ProjectConfig = json.RawMessage(cfg)
	return alias, nil
}

func (s *Store) ListAliases(ctx context.Context, projectID string) ([]domain.ProjectDeviceAlias, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT alias_id, project_id, logical_name, logical_type, target_ref, group_name, project_config_json
		FROM project_device_aliases WHERE project_id = ? ORDER BY logical_name, alias_id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list aliases: %w", err)
	}
	defer rows.Close()
	var aliases []domain.ProjectDeviceAlias
	for rows.Next() {
		var a domain.ProjectDeviceAlias
		var cfg string
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.LogicalName, &a.LogicalType, &a.TargetRef, &a.GroupName, &cfg); err != nil {
			return nil, fmt.Errorf("scan alias: %w", err)
		}
		a.ProjectConfig = json.RawMessage(cfg)
		aliases = append(aliases, a)
	}
	return aliases, rows.Err()
}
