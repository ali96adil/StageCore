package companion

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/companionchannel"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

const MachineRoleLogicalType = "machine_role"

type Forwarder struct {
	store            *store.Store
	channel          companionchannel.Channel
	heartbeatTimeout time.Duration
	now              func() time.Time
}

type machineRoleTargetConfig struct {
	MachineRoleID string `json:"machine_role_id"`
}

func NewForwarder(s *store.Store, channel companionchannel.Channel, heartbeatTimeout time.Duration, now func() time.Time) *Forwarder {
	if now == nil {
		now = time.Now
	}
	return &Forwarder{store: s, channel: channel, heartbeatTimeout: heartbeatTimeout, now: now}
}

func (f *Forwarder) Execute(ctx context.Context, req capability.Request) capability.Result {
	if f == nil || f.store == nil || f.channel == nil {
		return forwardFailure("COMPANION_FORWARDER_UNAVAILABLE", "Companion forwarding boundary is unavailable")
	}
	if req.Target == nil || !strings.EqualFold(strings.TrimSpace(req.Target.LogicalType), MachineRoleLogicalType) {
		return forwardFailure("COMPANION_TARGET_INVALID", "Companion forwarding requires a machine_role target")
	}
	if strings.TrimSpace(req.ExecutionID) == "" || strings.TrimSpace(req.RuntimeSnapshotID) == "" || strings.TrimSpace(req.Capability) == "" {
		return forwardFailure("COMPANION_REQUEST_INVALID", "execution id, Runtime Snapshot id and capability are required")
	}

	var targetConfig machineRoleTargetConfig
	if err := json.Unmarshal(req.Target.Configuration, &targetConfig); err != nil || strings.TrimSpace(targetConfig.MachineRoleID) == "" {
		return forwardFailure("MACHINE_ROLE_TARGET_INVALID", "machine_role target requires machine_role_id")
	}
	role, err := f.store.GetMachineRole(ctx, strings.TrimSpace(targetConfig.MachineRoleID))
	if err != nil {
		return forwardFailure("MACHINE_ROLE_NOT_FOUND", "machine role is not available")
	}
	if strings.TrimSpace(req.Target.Ref) != role.RoleKey {
		return forwardFailure("MACHINE_ROLE_TARGET_MISMATCH", "Snapshot target reference does not match Machine Role key")
	}

	runtimeSnapshot, err := f.store.GetRuntimeSnapshot(ctx, req.RuntimeSnapshotID)
	if err != nil || runtimeSnapshot.Status != domain.SnapshotPublished {
		return forwardFailure("SNAPSHOT_NOT_PUBLISHED", "published Runtime Snapshot is required for Companion execution")
	}
	if runtimeSnapshot.ProjectID != role.ProjectID {
		return forwardFailure("MACHINE_ROLE_PROJECT_MISMATCH", "Machine Role does not belong to the Runtime Snapshot Project")
	}
	if role.RequiredRuntimeSnapshotID != nil && *role.RequiredRuntimeSnapshotID != req.RuntimeSnapshotID {
		return forwardFailure("SNAPSHOT_MISMATCH", "runtime request does not match Machine Role Snapshot requirement")
	}

	assignment, err := f.store.GetActiveRoleAssignment(ctx, role.ID)
	if err != nil {
		return forwardFailure("MACHINE_ROLE_UNASSIGNED", "Machine Role has no active Companion assignment")
	}
	companionState, err := f.store.GetCompanion(ctx, assignment.CompanionID)
	if err != nil {
		return forwardFailure("COMPANION_UNAVAILABLE", "assigned Companion is not available")
	}

	evaluation := EvaluateRole(role, companionState, f.now().UTC(), f.heartbeatTimeout)
	if companionState.AppliedRuntimeSnapshotID == nil || *companionState.AppliedRuntimeSnapshotID != req.RuntimeSnapshotID {
		evaluation.Readiness = domain.CompanionReadinessMismatch
		evaluation.RoleState = domain.RoleMismatch
	}
	if err := f.store.SetRoleAssignmentState(ctx, assignment.ID, evaluation.RoleState); err != nil {
		return forwardFailure("ROLE_STATE_UPDATE_FAILED", "could not persist Machine Role readiness before execution")
	}
	if evaluation.RoleState != domain.RoleReady {
		return forwardFailure(readinessErrorCode(evaluation), readinessSummary(evaluation))
	}

	result := f.channel.Execute(ctx, companionchannel.ExecutionRequest{
		ExecutionID:       req.ExecutionID,
		CorrelationID:     req.CorrelationID,
		CompanionID:       assignment.CompanionID,
		MachineRoleID:     role.ID,
		RuntimeSnapshotID: req.RuntimeSnapshotID,
		Capability:        req.Capability,
		Parameters:        req.Parameters,
		TimeoutMS:         req.TimeoutMS,
	})
	if result.ErrorCode == "COMPANION_OFFLINE" {
		_ = f.store.SetRoleAssignmentState(context.WithoutCancel(ctx), assignment.ID, domain.RoleOffline)
	} else if result.ErrorCode == "SNAPSHOT_MISMATCH" {
		_ = f.store.SetRoleAssignmentState(context.WithoutCancel(ctx), assignment.ID, domain.RoleMismatch)
	}
	return capability.Result{
		Result:          result.Result,
		AckLevel:        result.AckLevel,
		ResponseSummary: result.ResponseSummary,
		ErrorCode:       result.ErrorCode,
	}
}

func readinessErrorCode(evaluation Evaluation) string {
	switch evaluation.RoleState {
	case domain.RoleOffline:
		return "COMPANION_OFFLINE"
	case domain.RoleMismatch:
		return "SNAPSHOT_MISMATCH"
	default:
		return "MACHINE_ROLE_NOT_READY"
	}
}

func readinessSummary(evaluation Evaluation) string {
	for _, check := range evaluation.Checks {
		if check.Status == CheckBlock {
			return check.Reason
		}
	}
	return fmt.Sprintf("Machine Role is %s", evaluation.RoleState)
}

func forwardFailure(code, summary string) capability.Result {
	return capability.Result{
		Result:          domain.ExecutionFailed,
		AckLevel:        contracts.AckNone,
		ErrorCode:       code,
		ResponseSummary: summary,
	}
}
