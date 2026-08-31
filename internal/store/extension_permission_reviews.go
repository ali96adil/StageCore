package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
)

const (
	ExtensionPermissionApproved = "APPROVED"
	ExtensionPermissionDenied   = "DENIED"
)

type ExtensionPermissionDecision struct {
	InstallationID string
	Permission     string
	Decision       string
	ReviewedBy     string
	ReviewedAt     time.Time
}

func (s *Store) SetExtensionPermissionDecision(ctx context.Context, installationID, permission, decision, actor string) (ExtensionPermissionDecision, error) {
	installationID = strings.TrimSpace(installationID)
	permission = strings.TrimSpace(permission)
	decision = strings.ToUpper(strings.TrimSpace(decision))
	actor = strings.TrimSpace(actor)
	if installationID == "" || permission == "" || actor == "" {
		return ExtensionPermissionDecision{}, fmt.Errorf("%w: installation ID, permission and actor are required", domain.ErrInvalidInput)
	}
	if decision != ExtensionPermissionApproved && decision != ExtensionPermissionDenied {
		return ExtensionPermissionDecision{}, fmt.Errorf("%w: unsupported extension permission decision", domain.ErrInvalidInput)
	}
	if _, err := s.GetExtensionInstallation(ctx, installationID); err != nil {
		return ExtensionPermissionDecision{}, err
	}
	now := s.clock.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO extension_permission_reviews (
			installation_id, permission, decision, reviewed_by, reviewed_at_us
		) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(installation_id, permission) DO UPDATE SET
			decision = excluded.decision,
			reviewed_by = excluded.reviewed_by,
			reviewed_at_us = excluded.reviewed_at_us
	`, installationID, permission, decision, actor, clock.UnixMicros(now)); err != nil {
		return ExtensionPermissionDecision{}, fmt.Errorf("set extension permission decision: %w", err)
	}
	return ExtensionPermissionDecision{
		InstallationID: installationID,
		Permission: permission,
		Decision: decision,
		ReviewedBy: actor,
		ReviewedAt: now,
	}, nil
}

func (s *Store) ListExtensionPermissionDecisions(ctx context.Context, installationID string) ([]ExtensionPermissionDecision, error) {
	installationID = strings.TrimSpace(installationID)
	if installationID == "" {
		return nil, fmt.Errorf("%w: installation ID is required", domain.ErrInvalidInput)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT installation_id, permission, decision, reviewed_by, reviewed_at_us
		FROM extension_permission_reviews
		WHERE installation_id = ?
		ORDER BY permission
	`, installationID)
	if err != nil {
		return nil, fmt.Errorf("list extension permission decisions: %w", err)
	}
	defer rows.Close()
	items := make([]ExtensionPermissionDecision, 0)
	for rows.Next() {
		var item ExtensionPermissionDecision
		var reviewedUS int64
		if err := rows.Scan(&item.InstallationID, &item.Permission, &item.Decision, &item.ReviewedBy, &reviewedUS); err != nil {
			return nil, fmt.Errorf("scan extension permission decision: %w", err)
		}
		item.ReviewedAt = clock.FromUnixMicros(reviewedUS)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate extension permission decisions: %w", err)
	}
	return items, nil
}
