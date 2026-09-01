package extension

import (
	"context"
	"fmt"
	"strings"
)

// PlanUpdate exposes the dependency-aware replacement plan through the
// supervised runtime boundary. Planning is read-only; execution still goes
// through UpdateInstallation so SHOW and runtime stop requirements are
// re-checked immediately before mutation.
func (s *RuntimeSupervisor) PlanUpdate(ctx context.Context, installationID, targetPackageID string) (UpdatePlan, error) {
	if s == nil || s.installer == nil {
		return UpdatePlan{}, fmt.Errorf("extension runtime supervisor is unavailable")
	}
	installationID = strings.TrimSpace(installationID)
	targetPackageID = strings.TrimSpace(targetPackageID)
	if installationID == "" || targetPackageID == "" {
		return UpdatePlan{}, fmt.Errorf("installation ID and target package ID are required")
	}
	return s.installer.PlanUpdate(ctx, installationID, targetPackageID)
}
