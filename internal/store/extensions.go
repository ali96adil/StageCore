package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
)

type ExtensionPackage struct {
	PackageID      string
	ExtensionID    string
	Version        string
	Kind           string
	Source         string
	ManifestJSON   json.RawMessage
	ManifestSHA256 string
	RegisteredBy   string
	RegisteredAt   time.Time
}

type RegisterExtensionPackageParams struct {
	PackageID      string
	ExtensionID    string
	Version        string
	Kind           string
	Source         string
	ManifestJSON   json.RawMessage
	ManifestSHA256 string
	RegisteredBy   string
}

func (s *Store) RegisterExtensionPackage(ctx context.Context, p RegisterExtensionPackageParams) (ExtensionPackage, error) {
	p.PackageID = strings.TrimSpace(p.PackageID)
	p.ExtensionID = strings.TrimSpace(p.ExtensionID)
	p.Version = strings.TrimSpace(p.Version)
	p.Kind = strings.ToUpper(strings.TrimSpace(p.Kind))
	p.Source = strings.ToUpper(strings.TrimSpace(p.Source))
	p.ManifestSHA256 = strings.ToLower(strings.TrimSpace(p.ManifestSHA256))
	p.RegisteredBy = strings.TrimSpace(p.RegisteredBy)
	if p.PackageID == "" || p.ExtensionID == "" || p.Version == "" || p.RegisteredBy == "" || len(p.ManifestSHA256) != 64 || !json.Valid(p.ManifestJSON) {
		return ExtensionPackage{}, fmt.Errorf("%w: extension package metadata is incomplete", domain.ErrInvalidInput)
	}
	if p.Kind != "PLUGIN" && p.Kind != "ADDON" {
		return ExtensionPackage{}, fmt.Errorf("%w: unsupported extension kind", domain.ErrInvalidInput)
	}
	if p.Source != "OFFICIAL" && p.Source != "LOCAL" && p.Source != "COMMUNITY" {
		return ExtensionPackage{}, fmt.Errorf("%w: unsupported extension source", domain.ErrInvalidInput)
	}
	var softwarePackageID string
	if err := s.db.QueryRowContext(ctx, `SELECT package_id FROM software_packages WHERE package_id = ?`, p.PackageID).Scan(&softwarePackageID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ExtensionPackage{}, domain.ErrNotFound
		}
		return ExtensionPackage{}, fmt.Errorf("read extension software package: %w", err)
	}
	now := s.clock.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO extension_packages (
			package_id, extension_id, version, kind, source,
			manifest_json, manifest_sha256, registered_by, registered_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.PackageID, p.ExtensionID, p.Version, p.Kind, p.Source,
		string(p.ManifestJSON), p.ManifestSHA256, p.RegisteredBy, clock.UnixMicros(now),
	); err != nil {
		return ExtensionPackage{}, fmt.Errorf("register extension package: %w", err)
	}
	return s.GetExtensionPackage(ctx, p.PackageID)
}

func (s *Store) GetExtensionPackage(ctx context.Context, packageID string) (ExtensionPackage, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT package_id, extension_id, version, kind, source,
		       manifest_json, manifest_sha256, registered_by, registered_at_us
		FROM extension_packages WHERE package_id = ?`, strings.TrimSpace(packageID))
	return scanExtensionPackage(row)
}

func (s *Store) ListExtensionPackages(ctx context.Context, extensionID string) ([]ExtensionPackage, error) {
	extensionID = strings.TrimSpace(extensionID)
	query := `
		SELECT package_id, extension_id, version, kind, source,
		       manifest_json, manifest_sha256, registered_by, registered_at_us
		FROM extension_packages`
	args := []any{}
	if extensionID != "" {
		query += ` WHERE extension_id = ?`
		args = append(args, extensionID)
	}
	query += ` ORDER BY extension_id, version DESC, registered_at_us DESC, package_id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list extension packages: %w", err)
	}
	defer rows.Close()
	items := make([]ExtensionPackage, 0)
	for rows.Next() {
		item, err := scanExtensionPackage(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate extension packages: %w", err)
	}
	return items, nil
}

func scanExtensionPackage(row rowScanner) (ExtensionPackage, error) {
	var item ExtensionPackage
	var manifest string
	var registeredUS int64
	if err := row.Scan(
		&item.PackageID, &item.ExtensionID, &item.Version, &item.Kind, &item.Source,
		&manifest, &item.ManifestSHA256, &item.RegisteredBy, &registeredUS,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ExtensionPackage{}, domain.ErrNotFound
		}
		return ExtensionPackage{}, fmt.Errorf("scan extension package: %w", err)
	}
	item.ManifestJSON = json.RawMessage(manifest)
	item.RegisteredAt = clock.FromUnixMicros(registeredUS)
	return item, nil
}
