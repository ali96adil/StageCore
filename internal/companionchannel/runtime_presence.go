package companionchannel

import (
	"context"
	"errors"

	"github.com/ali96adil/StageCore/internal/domain"
)

// updateRoleStateFromFreshReport derives role health only from a freshly
// authenticated Companion report. Runtime transport loss is handled
// separately and always forces OFFLINE while that connection generation is
// authoritative.
func (c *RuntimeChannel) updateRoleStateFromFreshReport(ctx context.Context, companion domain.Companion) error {
	assignment, err := c.store.GetActiveRoleAssignmentForCompanion(ctx, companion.ID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	role, err := c.store.GetMachineRole(ctx, assignment.MachineRoleID)
	if err != nil {
		return err
	}
	return c.store.SetRoleAssignmentState(ctx, assignment.ID, freshRuntimeRoleState(role, companion))
}

func freshRuntimeRoleState(role domain.MachineRole, companion domain.Companion) domain.RoleAssignmentState {
	if companion.Readiness == domain.CompanionReadinessOffline {
		return domain.RoleOffline
	}
	if companion.TrustState != domain.CompanionTrusted {
		return domain.RoleDegraded
	}
	if !hasRequiredRuntimeCapabilities(role.RequiredCapabilities, companion.Capabilities) {
		return domain.RoleDegraded
	}
	if role.RequiredRuntimeSnapshotID != nil {
		if companion.AppliedRuntimeSnapshotID == nil || *companion.AppliedRuntimeSnapshotID != *role.RequiredRuntimeSnapshotID {
			return domain.RoleMismatch
		}
	}
	if role.RequiredConfigHash != "" && companion.ConfigHash != role.RequiredConfigHash {
		return domain.RoleMismatch
	}

	switch companion.Readiness {
	case domain.CompanionReadinessReady:
		return domain.RoleReady
	case domain.CompanionReadinessSyncing:
		return domain.RoleSyncing
	case domain.CompanionReadinessMismatch:
		return domain.RoleMismatch
	case domain.CompanionReadinessDegraded, domain.CompanionReadinessBlocked:
		return domain.RoleDegraded
	default:
		return domain.RoleAssigned
	}
}

func hasRequiredRuntimeCapabilities(required, available []string) bool {
	if len(required) == 0 {
		return true
	}
	availableSet := make(map[string]struct{}, len(available))
	for _, capability := range available {
		availableSet[capability] = struct{}{}
	}
	for _, capability := range required {
		if _, ok := availableSet[capability]; !ok {
			return false
		}
	}
	return true
}
