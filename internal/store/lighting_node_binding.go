package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/lightingnode"
)

func (s *Store) SetLightingNodeBinding(ctx context.Context, revisionID string, binding lightingnode.ProjectBinding, updatedBy string) (lightingnode.ProjectBinding, error) {
	revisionID = strings.TrimSpace(revisionID)
	updatedBy = strings.TrimSpace(updatedBy)
	binding.DeviceID = strings.TrimSpace(binding.DeviceID)
	if binding.ProfileID == "" {
		binding.ProfileID = lightingnode.ProfileID
	}
	binding.ProfileID = strings.TrimSpace(binding.ProfileID)
	if revisionID == "" || updatedBy == "" || binding.DeviceID == "" {
		return lightingnode.ProjectBinding{}, domain.ErrInvalidInput
	}

	revision, err := s.GetRevision(ctx, revisionID)
	if err != nil {
		return lightingnode.ProjectBinding{}, err
	}
	if revision.Status != domain.RevisionDraft {
		return lightingnode.ProjectBinding{}, fmt.Errorf("%w: lighting configuration requires a DRAFT revision", domain.ErrRevisionFrozen)
	}
	if err := s.RequireProjectConfigurationMutable(ctx, revision.ProjectID); err != nil {
		return lightingnode.ProjectBinding{}, err
	}

	var deviceProjectID sql.NullString
	var profileID sql.NullString
	var enabled int
	err = s.db.QueryRowContext(ctx,
		"SELECT project_id, profile_id, enabled FROM stage_devices WHERE device_id = ?",
		binding.DeviceID,
	).Scan(&deviceProjectID, &profileID, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return lightingnode.ProjectBinding{}, domain.ErrNotFound
	}
	if err != nil {
		return lightingnode.ProjectBinding{}, fmt.Errorf("read lighting Stage Device: %w", err)
	}
	if enabled != 1 || !deviceProjectID.Valid || strings.TrimSpace(deviceProjectID.String) != revision.ProjectID {
		return lightingnode.ProjectBinding{}, fmt.Errorf("%w: lighting Stage Device must be enabled and belong to the same project", domain.ErrConflict)
	}
	if !profileID.Valid || strings.TrimSpace(profileID.String) != lightingnode.ProfileID {
		return lightingnode.ProjectBinding{}, fmt.Errorf("%w: Stage Device is not paired as profile %s", domain.ErrConflict, lightingnode.ProfileID)
	}

	binding.RevisionID = revision.ID
	binding.UpdatedBy = updatedBy
	if binding.Configuration.SchemaVersion == 0 {
		binding.Configuration.SchemaVersion = lightingnode.SchemaVersion1
	}
	if err := lightingnode.ValidateProjectBinding(binding); err != nil {
		return lightingnode.ProjectBinding{}, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
	}

	existing, err := s.ListLightingNodeBindings(ctx, revision.ID)
	if err != nil {
		return lightingnode.ProjectBinding{}, err
	}
	replaced := false
	for i := range existing {
		if existing[i].DeviceID == binding.DeviceID {
			existing[i] = binding
			replaced = true
			break
		}
	}
	if !replaced {
		existing = append(existing, binding)
	}
	if err := lightingnode.ValidateProjectBindings(existing); err != nil {
		return lightingnode.ProjectBinding{}, fmt.Errorf("%w: %v", domain.ErrConflict, err)
	}

	configJSON, err := lightingnode.CanonicalConfiguration(binding.Configuration)
	if err != nil {
		return lightingnode.ProjectBinding{}, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
	}
	aliasesJSON, err := json.Marshal(binding.Aliases)
	if err != nil {
		return lightingnode.ProjectBinding{}, fmt.Errorf("encode lighting aliases: %w", err)
	}
	nowUS := clock.UnixMicros(s.clock.Now().UTC())
	_, err = s.db.ExecContext(ctx,
		"INSERT INTO lighting_node_revision_bindings "+
			"(revision_id, device_id, profile_id, configuration_json, aliases_json, updated_by, updated_at_us) "+
			"VALUES (?, ?, ?, ?, ?, ?, ?) "+
			"ON CONFLICT(revision_id, device_id) DO UPDATE SET "+
			"profile_id=excluded.profile_id, configuration_json=excluded.configuration_json, aliases_json=excluded.aliases_json, "+
			"updated_by=excluded.updated_by, updated_at_us=excluded.updated_at_us",
		revision.ID, binding.DeviceID, binding.ProfileID, string(configJSON), string(aliasesJSON), updatedBy, nowUS,
	)
	if err != nil {
		return lightingnode.ProjectBinding{}, fmt.Errorf("persist lighting binding: %w", err)
	}
	return binding, nil
}

func (s *Store) ListLightingNodeBindings(ctx context.Context, revisionID string) ([]lightingnode.ProjectBinding, error) {
	revisionID = strings.TrimSpace(revisionID)
	if revisionID == "" {
		return nil, domain.ErrInvalidInput
	}
	if _, err := s.GetRevision(ctx, revisionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT device_id, profile_id, configuration_json, aliases_json, updated_by "+
			"FROM lighting_node_revision_bindings WHERE revision_id = ? ORDER BY device_id",
		revisionID,
	)
	if err != nil {
		return nil, fmt.Errorf("list lighting bindings: %w", err)
	}
	defer rows.Close()

	out := make([]lightingnode.ProjectBinding, 0)
	for rows.Next() {
		var binding lightingnode.ProjectBinding
		var configJSON, aliasesJSON string
		binding.RevisionID = revisionID
		if err := rows.Scan(&binding.DeviceID, &binding.ProfileID, &configJSON, &aliasesJSON, &binding.UpdatedBy); err != nil {
			return nil, fmt.Errorf("scan lighting binding: %w", err)
		}
		if err := json.Unmarshal([]byte(configJSON), &binding.Configuration); err != nil {
			return nil, fmt.Errorf("decode lighting configuration for %s: %w", binding.DeviceID, err)
		}
		if err := json.Unmarshal([]byte(aliasesJSON), &binding.Aliases); err != nil {
			return nil, fmt.Errorf("decode lighting aliases for %s: %w", binding.DeviceID, err)
		}
		if binding.Aliases == nil {
			binding.Aliases = map[string]string{}
		}
		if err := lightingnode.ValidateProjectBinding(binding); err != nil {
			return nil, fmt.Errorf("invalid persisted lighting binding for %s: %w", binding.DeviceID, err)
		}
		out = append(out, binding)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := lightingnode.ValidateProjectBindings(out); err != nil {
		return nil, fmt.Errorf("invalid persisted lighting bindings: %w", err)
	}
	return out, nil
}

func (s *Store) DeleteLightingNodeBinding(ctx context.Context, revisionID, deviceID string) error {
	revisionID = strings.TrimSpace(revisionID)
	deviceID = strings.TrimSpace(deviceID)
	if revisionID == "" || deviceID == "" {
		return domain.ErrInvalidInput
	}
	revision, err := s.GetRevision(ctx, revisionID)
	if err != nil {
		return err
	}
	if revision.Status != domain.RevisionDraft {
		return fmt.Errorf("%w: lighting configuration requires a DRAFT revision", domain.ErrRevisionFrozen)
	}
	if err := s.RequireProjectConfigurationMutable(ctx, revision.ProjectID); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx,
		"DELETE FROM lighting_node_revision_bindings WHERE revision_id = ? AND device_id = ?",
		revision.ID, deviceID,
	)
	if err != nil {
		return fmt.Errorf("delete lighting binding: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
