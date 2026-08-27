package companion_test

import (
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/companion"
	"github.com/ali96adil/StageCore/internal/domain"
)

func TestEvaluateRoleRequiresEvidenceBeforeReady(t *testing.T) {
	now := time.Date(2026, 8, 26, 17, 20, 0, 0, time.UTC)
	snapshotID := "11111111-1111-1111-1111-111111111111"
	role := domain.MachineRole{
		RoleKey:                   "VIDEO-MAIN",
		RequiredCapabilities:      []string{"osc.send"},
		RequiredRuntimeSnapshotID: &snapshotID,
		RequiredConfigHash:        "cfg-v1",
		Required:                  true,
	}
	companionState := domain.Companion{
		TrustState:               domain.CompanionTrusted,
		Capabilities:             []string{"osc.send"},
		LastSeenAt:               now.Add(-time.Second),
		Readiness:                domain.CompanionReadinessReady,
		AppliedRuntimeSnapshotID: &snapshotID,
		ConfigHash:               "cfg-v1",
	}
	evaluation := companion.EvaluateRole(role, companionState, now, 5*time.Second)
	if evaluation.Readiness != domain.CompanionReadinessReady || evaluation.RoleState != domain.RoleReady {
		t.Fatalf("evaluation=%#v", evaluation)
	}
	for _, check := range evaluation.Checks {
		if check.Status != companion.CheckPass {
			t.Fatalf("unexpected non-pass check: %#v", check)
		}
	}
}

func TestEvaluateRoleStaleHeartbeatIsOffline(t *testing.T) {
	now := time.Date(2026, 8, 26, 17, 21, 0, 0, time.UTC)
	evaluation := companion.EvaluateRole(
		domain.MachineRole{},
		domain.Companion{TrustState: domain.CompanionTrusted, LastSeenAt: now.Add(-6 * time.Second), Readiness: domain.CompanionReadinessReady},
		now,
		5*time.Second,
	)
	if evaluation.Readiness != domain.CompanionReadinessOffline || evaluation.RoleState != domain.RoleOffline {
		t.Fatalf("evaluation=%#v", evaluation)
	}
}

func TestEvaluateRoleMissingCapabilityBlocksReady(t *testing.T) {
	now := time.Date(2026, 8, 26, 17, 22, 0, 0, time.UTC)
	evaluation := companion.EvaluateRole(
		domain.MachineRole{RequiredCapabilities: []string{"osc.send"}},
		domain.Companion{TrustState: domain.CompanionTrusted, LastSeenAt: now, Readiness: domain.CompanionReadinessReady},
		now,
		5*time.Second,
	)
	if evaluation.Readiness != domain.CompanionReadinessBlocked || evaluation.RoleState != domain.RoleDegraded {
		t.Fatalf("evaluation=%#v", evaluation)
	}
}

func TestEvaluateRoleSnapshotMismatchIsExplicit(t *testing.T) {
	now := time.Date(2026, 8, 26, 17, 23, 0, 0, time.UTC)
	required := "11111111-1111-1111-1111-111111111111"
	applied := "22222222-2222-2222-2222-222222222222"
	evaluation := companion.EvaluateRole(
		domain.MachineRole{RequiredRuntimeSnapshotID: &required},
		domain.Companion{
			TrustState:               domain.CompanionTrusted,
			LastSeenAt:               now,
			Readiness:                domain.CompanionReadinessReady,
			AppliedRuntimeSnapshotID: &applied,
		},
		now,
		5*time.Second,
	)
	if evaluation.Readiness != domain.CompanionReadinessMismatch || evaluation.RoleState != domain.RoleMismatch {
		t.Fatalf("evaluation=%#v", evaluation)
	}
}
