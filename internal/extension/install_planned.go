package extension

import (
	"context"
	"fmt"

	"github.com/ali96adil/StageCore/internal/domain"
)

// InstallPlanned is the lifecycle-safe installation entrypoint. It refuses to
// materialize a root package until every required dependency is already
// installed at a compatible version. Install remains the lower-level verified
// storage materializer used by this package's crash/idempotency tests and by
// future plan executors after they have satisfied prerequisite ordering.
func (i *Installer) InstallPlanned(ctx context.Context, packageID, actor string) (Installation, error) {
	if i == nil || i.library == nil || i.library.store == nil {
		return Installation{}, fmt.Errorf("extension installer is unavailable")
	}
	activeType, err := i.library.store.ActiveOperationalSessionType(ctx)
	if err != nil {
		return Installation{}, err
	}
	if activeType == domain.SessionShow {
		return Installation{}, domain.ErrShowConfigurationLocked
	}

	plan, err := i.PlanInstall(ctx, packageID)
	if err != nil {
		return Installation{}, err
	}
	switch plan.Status {
	case InstallPlanReady:
		return i.Install(ctx, packageID, actor)
	case InstallPlanRequiresDependencies:
		return Installation{}, &InstallPlanError{Cause: ErrDependenciesRequired, Plan: plan}
	case InstallPlanBlocked:
		if len(plan.Blockers) != 0 && plan.Blockers[0].Code == "ROOT_VERSION_ALREADY_INSTALLED" {
			return Installation{}, ErrDifferentPackageInstalled
		}
		return Installation{}, &InstallPlanError{Cause: ErrDependencyPlanBlocked, Plan: plan}
	default:
		return Installation{}, &InstallPlanError{Cause: ErrDependencyPlanBlocked, Plan: plan}
	}
}
