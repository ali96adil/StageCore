package deviceexperience

import (
	"context"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/stagelaser"
)

func (r *Repository) validateStageLaserCommandAuthority(
	ctx context.Context,
	input CreateCommandInput,
	device Device,
) error {
	if stagelaser.CommandCapability(input.CommandType) == "" {
		return nil
	}
	if device.ProtocolVersion != ProtocolVersion2 ||
		device.Kind != DeviceGeneric ||
		strings.TrimSpace(device.ProfileID) != stagelaser.ProfileID {
		return fmt.Errorf("%w: StageLaser commands require authenticated v2 profile %s", ErrInvalidDevice, stagelaser.ProfileID)
	}
	if input.CommandType == stagelaser.CommandStateResync {
		activeShow, err := r.projectHasActiveShow(ctx, input.ProjectID)
		if err != nil {
			return err
		}
		if activeShow {
			return fmt.Errorf("%w: StageLaser resync is unavailable during an active SHOW", ErrInvalidState)
		}
	}
	var audited int
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM stage_device_assignments a
		JOIN stage_device_stagelaser_assignment_audit x
		  ON x.device_id=a.device_id AND x.to_epoch=a.assignment_epoch
		WHERE a.device_id=? AND a.assignment_state='ACTIVE'
		  AND a.project_id=? AND a.runtime_snapshot_id=?
		  AND x.project_id=a.project_id
		  AND x.runtime_snapshot_id=a.runtime_snapshot_id
	`, input.DeviceID, input.ProjectID, input.RuntimeSnapshotID).Scan(&audited); err != nil {
		return fmt.Errorf("verify StageLaser assignment audit: %w", err)
	}
	if audited != 1 {
		return fmt.Errorf("%w: StageLaser ACTIVE scope has no canonical safe-state assignment audit", ErrInvalidState)
	}
	return nil
}
