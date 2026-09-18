package deviceexperience

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/lightingnode"
)

func (r *Repository) validateLightingCommandAuthority(
	ctx context.Context,
	input CreateCommandInput,
	device Device,
	payload json.RawMessage,
) error {
	if lightingnode.CommandCapability(input.CommandType) == "" {
		return nil
	}
	if strings.TrimSpace(device.ProfileID) != lightingnode.ProfileID {
		return fmt.Errorf("%w: lighting commands require profile %s", ErrInvalidDevice, lightingnode.ProfileID)
	}

	switch input.CommandType {
	case lightingnode.CommandIdentify, lightingnode.CommandConfigApply:
		activeShow, err := r.projectHasActiveShow(ctx, input.ProjectID)
		if err != nil {
			return err
		}
		if activeShow {
			return fmt.Errorf("%w: %s is unavailable during an active SHOW", ErrInvalidState, input.CommandType)
		}
	}

	if input.CommandType != lightingnode.CommandConfigApply {
		return nil
	}

	snapshotID := strings.TrimSpace(input.RuntimeSnapshotID)
	if snapshotID == "" {
		return fmt.Errorf("%w: LIGHTING_CONFIG_APPLY requires a published Runtime Snapshot", ErrInvalidState)
	}

	var snapshotProjectID, snapshotStatus, manifestJSON string
	err := r.db.QueryRowContext(ctx,
		"SELECT project_id, status, manifest_json FROM runtime_snapshots WHERE runtime_snapshot_id = ?",
		snapshotID,
	).Scan(&snapshotProjectID, &snapshotStatus, &manifestJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: lighting Runtime Snapshot not found", ErrInvalidState)
	}
	if err != nil {
		return fmt.Errorf("read lighting Runtime Snapshot: %w", err)
	}
	if strings.TrimSpace(snapshotProjectID) != strings.TrimSpace(input.ProjectID) || snapshotStatus != "PUBLISHED" {
		return fmt.Errorf("%w: lighting configuration requires a published Runtime Snapshot for the same Project", ErrInvalidState)
	}

	var manifest struct {
		LightingNodes []lightingnode.ProjectBinding `json:"lighting_nodes"`
	}
	if err := json.Unmarshal([]byte(manifestJSON), &manifest); err != nil {
		return fmt.Errorf("%w: invalid lighting Runtime Snapshot manifest", ErrInvalidState)
	}
	var authoritative *lightingnode.ProjectBinding
	for i := range manifest.LightingNodes {
		if strings.TrimSpace(manifest.LightingNodes[i].DeviceID) == strings.TrimSpace(input.DeviceID) {
			copy := manifest.LightingNodes[i]
			authoritative = &copy
			break
		}
	}
	if authoritative == nil || strings.TrimSpace(authoritative.ProfileID) != lightingnode.ProfileID {
		return fmt.Errorf("%w: Runtime Snapshot has no authoritative lighting binding for device %s", ErrInvalidState, input.DeviceID)
	}

	var requested lightingnode.ConfigApplyPayload
	if err := json.Unmarshal(payload, &requested); err != nil {
		return fmt.Errorf("%w: invalid LIGHTING_CONFIG_APPLY payload", ErrInvalidState)
	}
	requestedCanonical, err := lightingnode.CanonicalConfiguration(requested.Configuration)
	if err != nil {
		return fmt.Errorf("%w: invalid requested lighting configuration", ErrInvalidState)
	}
	authoritativeCanonical, err := lightingnode.CanonicalConfiguration(authoritative.Configuration)
	if err != nil {
		return fmt.Errorf("%w: invalid authoritative lighting configuration", ErrInvalidState)
	}
	if !bytes.Equal(requestedCanonical, authoritativeCanonical) {
		return fmt.Errorf("%w: LIGHTING_CONFIG_APPLY payload does not match the published Runtime Snapshot", ErrInvalidState)
	}
	return nil
}

func (r *Repository) projectHasActiveShow(ctx context.Context, projectID string) (bool, error) {
	var count int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sessions WHERE project_id = ? AND session_type = 'SHOW' AND status = 'ACTIVE'",
		strings.TrimSpace(projectID),
	).Scan(&count); err != nil {
		return false, fmt.Errorf("check active SHOW for lighting command: %w", err)
	}
	return count > 0, nil
}
