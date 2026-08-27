package pluginpermissions

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Grant struct {
	PluginID   string    `json:"plugin_id"`
	Permission string    `json:"permission"`
	Granted    bool      `json:"granted"`
	UpdatedAt  time.Time `json:"updated_at"`
	UpdatedBy  string    `json:"updated_by"`
}

type Service struct {
	db  *sql.DB
	now func() time.Time
}

type Option func(*Service)

func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func New(database *sql.DB, options ...Option) (*Service, error) {
	if database == nil {
		return nil, fmt.Errorf("database is required")
	}
	s := &Service{db: database, now: time.Now}
	for _, option := range options {
		option(s)
	}
	return s, nil
}

func (s *Service) Set(ctx context.Context, pluginID, permission string, granted bool, actor string) (Grant, error) {
	pluginID = strings.TrimSpace(pluginID)
	permission = strings.TrimSpace(permission)
	actor = strings.TrimSpace(actor)
	if pluginID == "" || permission == "" || actor == "" {
		return Grant{}, fmt.Errorf("plugin ID, permission and actor are required")
	}
	now := s.now().UTC()
	value := 0
	if granted {
		value = 1
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO plugin_permission_grants (plugin_id, permission, granted, updated_at_us, updated_by)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(plugin_id, permission) DO UPDATE SET
		granted = excluded.granted,
		updated_at_us = excluded.updated_at_us,
		updated_by = excluded.updated_by
	`, pluginID, permission, value, now.UnixMicro(), actor); err != nil {
		return Grant{}, fmt.Errorf("set plugin permission: %w", err)
	}
	return Grant{PluginID: pluginID, Permission: permission, Granted: granted, UpdatedAt: now, UpdatedBy: actor}, nil
}

func (s *Service) List(ctx context.Context, pluginID string) ([]Grant, error) {
	pluginID = strings.TrimSpace(pluginID)
	query := `SELECT plugin_id, permission, granted, updated_at_us, updated_by FROM plugin_permission_grants`
	args := []any{}
	if pluginID != "" {
		query += ` WHERE plugin_id = ?`
		args = append(args, pluginID)
	}
	query += ` ORDER BY plugin_id, permission`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list plugin permissions: %w", err)
	}
	defer rows.Close()
	var grants []Grant
	for rows.Next() {
		var grant Grant
		var granted int
		var updatedUS int64
		if err := rows.Scan(&grant.PluginID, &grant.Permission, &granted, &updatedUS, &grant.UpdatedBy); err != nil {
			return nil, fmt.Errorf("scan plugin permission: %w", err)
		}
		grant.Granted = granted == 1
		grant.UpdatedAt = time.UnixMicro(updatedUS).UTC()
		grants = append(grants, grant)
	}
	return grants, rows.Err()
}

func (s *Service) Granted(ctx context.Context, pluginID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT permission FROM plugin_permission_grants
		WHERE plugin_id = ? AND granted = 1
		ORDER BY permission
	`, strings.TrimSpace(pluginID))
	if err != nil {
		return nil, fmt.Errorf("read granted plugin permissions: %w", err)
	}
	defer rows.Close()
	var permissions []string
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			return nil, err
		}
		permissions = append(permissions, permission)
	}
	return permissions, rows.Err()
}
