package store

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

const ExtensionInstallationInstalled = "INSTALLED"

type ExtensionInstallation struct {
	InstallationID      string
	PackageID           string
	LifecycleState      string
	PayloadRelativePath string
	ContentSHA256       string
	SizeBytes           int64
	InstalledBy         string
	InstalledAt         time.Time
}

type RegisterExtensionInstallationParams struct {
	PackageID           string
	LifecycleState      string
	PayloadRelativePath string
	ContentSHA256       string
	SizeBytes           int64
	InstalledBy         string
}

func normalizeExtensionInstallationParams(p RegisterExtensionInstallationParams) (RegisterExtensionInstallationParams, error) {
	p.PackageID = strings.TrimSpace(p.PackageID)
	p.LifecycleState = strings.ToUpper(strings.TrimSpace(p.LifecycleState))
	p.PayloadRelativePath = path.Clean(strings.TrimSpace(p.PayloadRelativePath))
	p.ContentSHA256 = strings.ToLower(strings.TrimSpace(p.ContentSHA256))
	p.InstalledBy = strings.TrimSpace(p.InstalledBy)
	if p.PackageID == "" || p.InstalledBy == "" {
		return RegisterExtensionInstallationParams{}, fmt.Errorf("%w: package ID and installation actor are required", domain.ErrInvalidInput)
	}
	if p.LifecycleState != ExtensionInstallationInstalled {
		return RegisterExtensionInstallationParams{}, fmt.Errorf("%w: unsupported extension installation lifecycle state", domain.ErrInvalidInput)
	}
	if p.PayloadRelativePath == "." || path.IsAbs(p.PayloadRelativePath) || p.PayloadRelativePath == ".." || strings.HasPrefix(p.PayloadRelativePath, "../") {
		return RegisterExtensionInstallationParams{}, fmt.Errorf("%w: installed payload path must be a safe relative path", domain.ErrInvalidInput)
	}
	hash, err := hex.DecodeString(p.ContentSHA256)
	if err != nil || len(hash) != 32 {
		return RegisterExtensionInstallationParams{}, fmt.Errorf("%w: installed payload SHA-256 is invalid", domain.ErrInvalidInput)
	}
	if p.SizeBytes < 0 {
		return RegisterExtensionInstallationParams{}, fmt.Errorf("%w: installed payload size cannot be negative", domain.ErrInvalidInput)
	}
	return p, nil
}

func (s *Store) RegisterExtensionInstallation(ctx context.Context, p RegisterExtensionInstallationParams) (ExtensionInstallation, error) {
	var err error
	p, err = normalizeExtensionInstallationParams(p)
	if err != nil {
		return ExtensionInstallation{}, err
	}
	if _, err := s.GetExtensionPackage(ctx, p.PackageID); err != nil {
		return ExtensionInstallation{}, fmt.Errorf("extension package: %w", err)
	}

	installationID, err := stageid.New()
	if err != nil {
		return ExtensionInstallation{}, err
	}
	now := s.clock.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO extension_installations (
			installation_id, package_id, lifecycle_state, payload_relative_path,
			content_sha256, size_bytes, installed_by, installed_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		installationID, p.PackageID, p.LifecycleState, p.PayloadRelativePath,
		p.ContentSHA256, p.SizeBytes, p.InstalledBy, clock.UnixMicros(now),
	)
	if err != nil {
		return ExtensionInstallation{}, fmt.Errorf("register extension installation: %w", err)
	}
	return s.GetExtensionInstallation(ctx, installationID)
}

// ReplaceExtensionInstallation atomically moves one durable installation to a
// different immutable package. Permission reviews and runtime intent are tied
// to the exact installed artifact, so they are deliberately cleared instead
// of being inherited by the replacement version.
func (s *Store) ReplaceExtensionInstallation(ctx context.Context, installationID string, p RegisterExtensionInstallationParams) (ExtensionInstallation, error) {
	installationID = strings.TrimSpace(installationID)
	if installationID == "" {
		return ExtensionInstallation{}, fmt.Errorf("%w: installation ID is required", domain.ErrInvalidInput)
	}
	var err error
	p, err = normalizeExtensionInstallationParams(p)
	if err != nil {
		return ExtensionInstallation{}, err
	}
	if _, err := s.GetExtensionInstallation(ctx, installationID); err != nil {
		return ExtensionInstallation{}, err
	}
	if _, err := s.GetExtensionPackage(ctx, p.PackageID); err != nil {
		return ExtensionInstallation{}, fmt.Errorf("extension package: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ExtensionInstallation{}, fmt.Errorf("begin extension installation replacement: %w", err)
	}
	defer tx.Rollback()
	for _, query := range []string{
		`DELETE FROM extension_permission_reviews WHERE installation_id = ?`,
		`DELETE FROM extension_runtime_lifecycle WHERE installation_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, query, installationID); err != nil {
			return ExtensionInstallation{}, fmt.Errorf("clear extension replacement state: %w", err)
		}
	}
	now := s.clock.Now().UTC()
	result, err := tx.ExecContext(ctx, `
		UPDATE extension_installations
		SET package_id = ?, lifecycle_state = ?, payload_relative_path = ?,
		    content_sha256 = ?, size_bytes = ?, installed_by = ?, installed_at_us = ?
		WHERE installation_id = ?`,
		p.PackageID, p.LifecycleState, p.PayloadRelativePath,
		p.ContentSHA256, p.SizeBytes, p.InstalledBy, clock.UnixMicros(now), installationID,
	)
	if err != nil {
		return ExtensionInstallation{}, fmt.Errorf("replace extension installation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return ExtensionInstallation{}, fmt.Errorf("replace extension installation rows affected: %w", err)
	}
	if rows != 1 {
		return ExtensionInstallation{}, domain.ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return ExtensionInstallation{}, fmt.Errorf("commit extension installation replacement: %w", err)
	}
	return s.GetExtensionInstallation(ctx, installationID)
}

func (s *Store) GetExtensionInstallation(ctx context.Context, installationID string) (ExtensionInstallation, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT installation_id, package_id, lifecycle_state, payload_relative_path,
		       content_sha256, size_bytes, installed_by, installed_at_us
		FROM extension_installations
		WHERE installation_id = ?`, strings.TrimSpace(installationID))
	return scanExtensionInstallation(row)
}

func (s *Store) GetExtensionInstallationByPackageID(ctx context.Context, packageID string) (ExtensionInstallation, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT installation_id, package_id, lifecycle_state, payload_relative_path,
		       content_sha256, size_bytes, installed_by, installed_at_us
		FROM extension_installations
		WHERE package_id = ?`, strings.TrimSpace(packageID))
	return scanExtensionInstallation(row)
}

func (s *Store) ListExtensionInstallations(ctx context.Context, extensionID string) ([]ExtensionInstallation, error) {
	extensionID = strings.TrimSpace(extensionID)
	query := `
		SELECT ei.installation_id, ei.package_id, ei.lifecycle_state, ei.payload_relative_path,
		       ei.content_sha256, ei.size_bytes, ei.installed_by, ei.installed_at_us
		FROM extension_installations ei
		JOIN extension_packages ep ON ep.package_id = ei.package_id`
	args := []any{}
	if extensionID != "" {
		query += ` WHERE ep.extension_id = ?`
		args = append(args, extensionID)
	}
	query += ` ORDER BY ei.installed_at_us DESC, ei.installation_id DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list extension installations: %w", err)
	}
	defer rows.Close()
	items := make([]ExtensionInstallation, 0)
	for rows.Next() {
		item, err := scanExtensionInstallation(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate extension installations: %w", err)
	}
	return items, nil
}

func scanExtensionInstallation(row rowScanner) (ExtensionInstallation, error) {
	var item ExtensionInstallation
	var installedUS int64
	if err := row.Scan(
		&item.InstallationID, &item.PackageID, &item.LifecycleState, &item.PayloadRelativePath,
		&item.ContentSHA256, &item.SizeBytes, &item.InstalledBy, &installedUS,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ExtensionInstallation{}, domain.ErrNotFound
		}
		return ExtensionInstallation{}, fmt.Errorf("scan extension installation: %w", err)
	}
	item.InstalledAt = clock.FromUnixMicros(installedUS)
	return item, nil
}
