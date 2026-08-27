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
	MediaPolicyReferenceOnly   = "REFERENCE_ONLY"
	MediaPolicyManaged         = "MANAGED"
	MediaPolicyArchiveRequired = "ARCHIVE_REQUIRED"
)

type VaultObject struct {
	ContentHash  string
	SizeBytes    int64
	RelativePath string
	CreatedAt    time.Time
}

type MediaAsset struct {
	ID          string
	ProjectID   string
	Name        string
	AssetPolicy string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type MediaContentVersion struct {
	ID               string
	MediaAssetID     string
	ContentHash      string
	OriginalFilename string
	SizeBytes        int64
	CreatedAt        time.Time
}

type MediaLocation struct {
	ID               string
	ContentVersionID string
	LocationType     string
	Locator          string
	Status           string
	VerifiedAt       *time.Time
}

type ManagedMedia struct {
	Object   VaultObject
	Asset    MediaAsset
	Version  MediaContentVersion
	Location MediaLocation
}

type MachineRoleMediaRequirement struct {
	ID               string
	MachineRoleID    string
	RoleKey          string
	MediaAssetID     string
	ContentVersionID string
	ContentHash      string
	SizeBytes        int64
	Required         bool
	CreatedAt        time.Time
}

type RegisterManagedMediaParams struct {
	ProjectID        string
	Name             string
	AssetPolicy      string
	ContentHash      string
	SizeBytes        int64
	RelativePath     string
	OriginalFilename string
}

func (s *Store) RegisterManagedMedia(ctx context.Context, p RegisterManagedMediaParams) (ManagedMedia, error) {
	p.ProjectID = strings.TrimSpace(p.ProjectID)
	p.Name = strings.TrimSpace(p.Name)
	p.AssetPolicy = strings.TrimSpace(p.AssetPolicy)
	p.ContentHash = strings.ToLower(strings.TrimSpace(p.ContentHash))
	p.RelativePath = strings.TrimSpace(p.RelativePath)
	p.OriginalFilename = strings.TrimSpace(p.OriginalFilename)
	if p.ProjectID == "" || p.Name == "" || p.ContentHash == "" || p.RelativePath == "" {
		return ManagedMedia{}, fmt.Errorf("%w: project, name, content hash and relative path are required", domain.ErrInvalidInput)
	}
	if p.AssetPolicy == "" {
		p.AssetPolicy = MediaPolicyManaged
	}
	if p.AssetPolicy != MediaPolicyManaged && p.AssetPolicy != MediaPolicyArchiveRequired {
		return ManagedMedia{}, fmt.Errorf("%w: managed import requires MANAGED or ARCHIVE_REQUIRED policy", domain.ErrInvalidInput)
	}
	if len(p.ContentHash) != 64 {
		return ManagedMedia{}, fmt.Errorf("%w: SHA-256 content hash must be 64 hex characters", domain.ErrInvalidInput)
	}
	if p.SizeBytes < 0 {
		return ManagedMedia{}, fmt.Errorf("%w: size cannot be negative", domain.ErrInvalidInput)
	}
	if _, err := s.GetProject(ctx, p.ProjectID); err != nil {
		return ManagedMedia{}, err
	}

	assetID, err := stageid.New()
	if err != nil {
		return ManagedMedia{}, err
	}
	versionID, err := stageid.New()
	if err != nil {
		return ManagedMedia{}, err
	}
	locationID, err := stageid.New()
	if err != nil {
		return ManagedMedia{}, err
	}
	now := s.clock.Now().UTC()
	nowUS := clock.UnixMicros(now)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ManagedMedia{}, fmt.Errorf("begin managed media registration: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO vault_objects (content_hash, size_bytes, relative_path, created_at_us)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(content_hash) DO NOTHING`, p.ContentHash, p.SizeBytes, p.RelativePath, nowUS); err != nil {
		return ManagedMedia{}, fmt.Errorf("register Vault object: %w", err)
	}
	var existingSize int64
	var existingPath string
	var objectCreatedUS int64
	if err := tx.QueryRowContext(ctx, `
		SELECT size_bytes, relative_path, created_at_us FROM vault_objects WHERE content_hash = ?`, p.ContentHash).
		Scan(&existingSize, &existingPath, &objectCreatedUS); err != nil {
		return ManagedMedia{}, fmt.Errorf("read Vault object after register: %w", err)
	}
	if existingSize != p.SizeBytes || existingPath != p.RelativePath {
		return ManagedMedia{}, fmt.Errorf("%w: existing Vault identity metadata conflicts with verified content", domain.ErrInvalidInput)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO media_assets (media_asset_id, project_id, name, asset_policy, created_at_us, updated_at_us)
		VALUES (?, ?, ?, ?, ?, ?)`, assetID, p.ProjectID, p.Name, p.AssetPolicy, nowUS, nowUS); err != nil {
		return ManagedMedia{}, fmt.Errorf("create media asset: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO media_content_versions (content_version_id, media_asset_id, content_hash, original_filename, size_bytes, created_at_us)
		VALUES (?, ?, ?, ?, ?, ?)`, versionID, assetID, p.ContentHash, p.OriginalFilename, p.SizeBytes, nowUS); err != nil {
		return ManagedMedia{}, fmt.Errorf("create media content version: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO media_locations (media_location_id, content_version_id, location_type, locator, status, verified_at_us)
		VALUES (?, ?, 'HUB', ?, 'AVAILABLE', ?)`, locationID, versionID, p.RelativePath, nowUS); err != nil {
		return ManagedMedia{}, fmt.Errorf("create Hub media location: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ManagedMedia{}, fmt.Errorf("commit managed media registration: %w", err)
	}

	verified := now
	return ManagedMedia{
		Object: VaultObject{ContentHash: p.ContentHash, SizeBytes: p.SizeBytes, RelativePath: p.RelativePath, CreatedAt: clock.FromUnixMicros(objectCreatedUS)},
		Asset: MediaAsset{ID: assetID, ProjectID: p.ProjectID, Name: p.Name, AssetPolicy: p.AssetPolicy, CreatedAt: now, UpdatedAt: now},
		Version: MediaContentVersion{ID: versionID, MediaAssetID: assetID, ContentHash: p.ContentHash, OriginalFilename: p.OriginalFilename, SizeBytes: p.SizeBytes, CreatedAt: now},
		Location: MediaLocation{ID: locationID, ContentVersionID: versionID, LocationType: "HUB", Locator: p.RelativePath, Status: "AVAILABLE", VerifiedAt: &verified},
	}, nil
}

func (s *Store) AddMachineRoleMediaRequirement(ctx context.Context, machineRoleID, contentVersionID string, required bool) (MachineRoleMediaRequirement, error) {
	machineRoleID = strings.TrimSpace(machineRoleID)
	contentVersionID = strings.TrimSpace(contentVersionID)
	if machineRoleID == "" || contentVersionID == "" {
		return MachineRoleMediaRequirement{}, fmt.Errorf("%w: machine role and content version are required", domain.ErrInvalidInput)
	}
	role, err := s.GetMachineRole(ctx, machineRoleID)
	if err != nil {
		return MachineRoleMediaRequirement{}, err
	}
	var assetProjectID string
	if err := s.db.QueryRowContext(ctx, `
		SELECT a.project_id
		FROM media_content_versions v
		JOIN media_assets a ON a.media_asset_id = v.media_asset_id
		WHERE v.content_version_id = ?`, contentVersionID).Scan(&assetProjectID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MachineRoleMediaRequirement{}, domain.ErrNotFound
		}
		return MachineRoleMediaRequirement{}, fmt.Errorf("read media content version project: %w", err)
	}
	if assetProjectID != role.ProjectID {
		return MachineRoleMediaRequirement{}, fmt.Errorf("%w: media requirement must belong to the Machine Role project", domain.ErrConflict)
	}
	id, err := stageid.New()
	if err != nil {
		return MachineRoleMediaRequirement{}, err
	}
	now := s.clock.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO machine_role_media_requirements (
			media_requirement_id, machine_role_id, content_version_id, required, created_at_us
		) VALUES (?, ?, ?, ?, ?)`, id, machineRoleID, contentVersionID, boolInt(required), clock.UnixMicros(now)); err != nil {
		return MachineRoleMediaRequirement{}, fmt.Errorf("add Machine Role media requirement: %w", err)
	}
	items, err := s.ListProjectMediaRequirements(ctx, role.ProjectID)
	if err != nil {
		return MachineRoleMediaRequirement{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return MachineRoleMediaRequirement{}, domain.ErrNotFound
}

func (s *Store) ListProjectMediaRequirements(ctx context.Context, projectID string) ([]MachineRoleMediaRequirement, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.media_requirement_id, r.machine_role_id, mr.role_key,
		       a.media_asset_id, v.content_version_id, v.content_hash, v.size_bytes,
		       r.required, r.created_at_us
		FROM machine_role_media_requirements r
		JOIN machine_roles mr ON mr.machine_role_id = r.machine_role_id
		JOIN media_content_versions v ON v.content_version_id = r.content_version_id
		JOIN media_assets a ON a.media_asset_id = v.media_asset_id
		WHERE mr.project_id = ?
		ORDER BY mr.role_key, a.media_asset_id, v.content_version_id`, strings.TrimSpace(projectID))
	if err != nil {
		return nil, fmt.Errorf("list project media requirements: %w", err)
	}
	defer rows.Close()
	items := make([]MachineRoleMediaRequirement, 0)
	for rows.Next() {
		var item MachineRoleMediaRequirement
		var required int
		var createdUS int64
		if err := rows.Scan(
			&item.ID, &item.MachineRoleID, &item.RoleKey,
			&item.MediaAssetID, &item.ContentVersionID, &item.ContentHash, &item.SizeBytes,
			&required, &createdUS,
		); err != nil {
			return nil, fmt.Errorf("scan project media requirement: %w", err)
		}
		item.Required = required == 1
		item.CreatedAt = clock.FromUnixMicros(createdUS)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project media requirements: %w", err)
	}
	return items, nil
}

func (s *Store) GetVaultObject(ctx context.Context, contentHash string) (VaultObject, error) {
	var object VaultObject
	var createdUS int64
	err := s.db.QueryRowContext(ctx, `
		SELECT content_hash, size_bytes, relative_path, created_at_us
		FROM vault_objects WHERE content_hash = ?`, strings.ToLower(strings.TrimSpace(contentHash))).Scan(
		&object.ContentHash, &object.SizeBytes, &object.RelativePath, &createdUS,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return VaultObject{}, domain.ErrNotFound
	}
	if err != nil {
		return VaultObject{}, fmt.Errorf("get Vault object: %w", err)
	}
	object.CreatedAt = clock.FromUnixMicros(createdUS)
	return object, nil
}

func (s *Store) GetMediaAsset(ctx context.Context, assetID string) (MediaAsset, error) {
	var asset MediaAsset
	var createdUS, updatedUS int64
	err := s.db.QueryRowContext(ctx, `
		SELECT media_asset_id, project_id, name, asset_policy, created_at_us, updated_at_us
		FROM media_assets WHERE media_asset_id = ?`, assetID).Scan(
		&asset.ID, &asset.ProjectID, &asset.Name, &asset.AssetPolicy, &createdUS, &updatedUS,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return MediaAsset{}, domain.ErrNotFound
	}
	if err != nil {
		return MediaAsset{}, fmt.Errorf("get media asset: %w", err)
	}
	asset.CreatedAt = clock.FromUnixMicros(createdUS)
	asset.UpdatedAt = clock.FromUnixMicros(updatedUS)
	return asset, nil
}
