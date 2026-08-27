package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

const (
	SoftwareSigningUnknown  = "UNKNOWN"
	SoftwareSigningUnsigned = "UNSIGNED"
	SoftwareSigningSigned   = "SIGNED"

	SoftwareNotarizationUnknown       = "UNKNOWN"
	SoftwareNotarizationNotApplicable = "NOT_APPLICABLE"
	SoftwareNotarizationNotNotarized  = "NOT_NOTARIZED"
	SoftwareNotarizationNotarized     = "NOTARIZED"

	SoftwareChannelDevelopment = "development"
	SoftwareChannelRelease     = "release"
)

type SoftwarePackage struct {
	ID                 string
	ProductID          string
	Version            string
	Platform           string
	Architecture       string
	MinAPIVersion      int
	MaxAPIVersion      int
	ContentHash        string
	SizeBytes          int64
	OriginalFilename   string
	SigningStatus      string
	NotarizationStatus string
	ReleaseChannel     string
	ReleaseNotes       string
	CreatedAt          time.Time
}

type RegisterSoftwarePackageParams struct {
	ProductID          string
	Version            string
	Platform           string
	Architecture       string
	MinAPIVersion      int
	MaxAPIVersion      int
	ContentHash        string
	SizeBytes          int64
	OriginalFilename   string
	SigningStatus      string
	NotarizationStatus string
	ReleaseChannel     string
	ReleaseNotes       string
}

func (s *Store) RegisterSoftwarePackage(ctx context.Context, p RegisterSoftwarePackageParams) (SoftwarePackage, error) {
	p.ProductID = strings.TrimSpace(p.ProductID)
	p.Version = strings.TrimSpace(p.Version)
	p.Platform = strings.ToLower(strings.TrimSpace(p.Platform))
	p.Architecture = strings.ToLower(strings.TrimSpace(p.Architecture))
	p.ContentHash = strings.ToLower(strings.TrimSpace(p.ContentHash))
	p.OriginalFilename = strings.TrimSpace(p.OriginalFilename)
	p.SigningStatus = strings.ToUpper(strings.TrimSpace(p.SigningStatus))
	p.NotarizationStatus = strings.ToUpper(strings.TrimSpace(p.NotarizationStatus))
	p.ReleaseChannel = strings.ToLower(strings.TrimSpace(p.ReleaseChannel))

	if p.ProductID == "" || p.Version == "" || p.Platform == "" || p.Architecture == "" || len(p.ContentHash) != 64 {
		return SoftwarePackage{}, fmt.Errorf("%w: product, version, platform, architecture and SHA-256 content hash are required", domain.ErrInvalidInput)
	}
	if p.MinAPIVersion < 0 || p.MaxAPIVersion < p.MinAPIVersion {
		return SoftwarePackage{}, fmt.Errorf("%w: invalid API compatibility range", domain.ErrInvalidInput)
	}
	if p.SizeBytes < 0 {
		return SoftwarePackage{}, fmt.Errorf("%w: package size cannot be negative", domain.ErrInvalidInput)
	}
	if p.SigningStatus == "" {
		p.SigningStatus = SoftwareSigningUnknown
	}
	if p.NotarizationStatus == "" {
		p.NotarizationStatus = SoftwareNotarizationUnknown
	}
	if p.ReleaseChannel == "" {
		p.ReleaseChannel = SoftwareChannelDevelopment
	}
	if !validSoftwareSigningStatus(p.SigningStatus) || !validSoftwareNotarizationStatus(p.NotarizationStatus) || !validSoftwareChannel(p.ReleaseChannel) {
		return SoftwarePackage{}, fmt.Errorf("%w: invalid software package status metadata", domain.ErrInvalidInput)
	}
	object, err := s.GetVaultObject(ctx, p.ContentHash)
	if err != nil {
		return SoftwarePackage{}, fmt.Errorf("software package Vault object: %w", err)
	}
	if object.SizeBytes != p.SizeBytes {
		return SoftwarePackage{}, fmt.Errorf("%w: software package size does not match Vault object", domain.ErrInvalidInput)
	}

	packageID, err := stageid.New()
	if err != nil {
		return SoftwarePackage{}, err
	}
	now := s.clock.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO software_packages (
			package_id, product_id, version, platform, architecture,
			min_api_version, max_api_version, content_hash, size_bytes, original_filename,
			signing_status, notarization_status, release_channel, release_notes, created_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		packageID, p.ProductID, p.Version, p.Platform, p.Architecture,
		p.MinAPIVersion, p.MaxAPIVersion, p.ContentHash, p.SizeBytes, p.OriginalFilename,
		p.SigningStatus, p.NotarizationStatus, p.ReleaseChannel, p.ReleaseNotes, clock.UnixMicros(now),
	)
	if err != nil {
		return SoftwarePackage{}, fmt.Errorf("register software package: %w", err)
	}
	return s.GetSoftwarePackage(ctx, packageID)
}

func (s *Store) GetSoftwarePackage(ctx context.Context, packageID string) (SoftwarePackage, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT package_id, product_id, version, platform, architecture,
		       min_api_version, max_api_version, content_hash, size_bytes, original_filename,
		       signing_status, notarization_status, release_channel, release_notes, created_at_us
		FROM software_packages WHERE package_id = ?`, strings.TrimSpace(packageID))
	return scanSoftwarePackage(row)
}

func (s *Store) ListSoftwarePackages(ctx context.Context, productID string) ([]SoftwarePackage, error) {
	productID = strings.TrimSpace(productID)
	query := `
		SELECT package_id, product_id, version, platform, architecture,
		       min_api_version, max_api_version, content_hash, size_bytes, original_filename,
		       signing_status, notarization_status, release_channel, release_notes, created_at_us
		FROM software_packages`
	args := []any{}
	if productID != "" {
		query += ` WHERE product_id = ?`
		args = append(args, productID)
	}
	query += ` ORDER BY created_at_us DESC, package_id DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list software packages: %w", err)
	}
	defer rows.Close()
	packages := make([]SoftwarePackage, 0)
	for rows.Next() {
		pkg, err := scanSoftwarePackage(rows)
		if err != nil {
			return nil, err
		}
		packages = append(packages, pkg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate software packages: %w", err)
	}
	return packages, nil
}

func scanSoftwarePackage(row rowScanner) (SoftwarePackage, error) {
	var pkg SoftwarePackage
	var createdUS int64
	if err := row.Scan(
		&pkg.ID, &pkg.ProductID, &pkg.Version, &pkg.Platform, &pkg.Architecture,
		&pkg.MinAPIVersion, &pkg.MaxAPIVersion, &pkg.ContentHash, &pkg.SizeBytes, &pkg.OriginalFilename,
		&pkg.SigningStatus, &pkg.NotarizationStatus, &pkg.ReleaseChannel, &pkg.ReleaseNotes, &createdUS,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SoftwarePackage{}, domain.ErrNotFound
		}
		return SoftwarePackage{}, fmt.Errorf("scan software package: %w", err)
	}
	pkg.CreatedAt = clock.FromUnixMicros(createdUS)
	return pkg, nil
}

func validSoftwareSigningStatus(value string) bool {
	switch value {
	case SoftwareSigningUnknown, SoftwareSigningUnsigned, SoftwareSigningSigned:
		return true
	default:
		return false
	}
}

func validSoftwareNotarizationStatus(value string) bool {
	switch value {
	case SoftwareNotarizationUnknown, SoftwareNotarizationNotApplicable, SoftwareNotarizationNotNotarized, SoftwareNotarizationNotarized:
		return true
	default:
		return false
	}
}

func validSoftwareChannel(value string) bool {
	return value == SoftwareChannelDevelopment || value == SoftwareChannelRelease
}
