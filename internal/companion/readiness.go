package companion

import (
	"fmt"
	"sort"
	"time"

	"github.com/ali96adil/StageCore/internal/domain"
)

type CheckStatus string

const (
	CheckPass  CheckStatus = "PASS"
	CheckWarn  CheckStatus = "WARN"
	CheckBlock CheckStatus = "BLOCK"
)

type Check struct {
	Key    string
	Status CheckStatus
	Reason string
}

type Evaluation struct {
	Readiness domain.CompanionReadiness
	RoleState domain.RoleAssignmentState
	Checks    []Check
}

func EvaluateRole(role domain.MachineRole, c domain.Companion, now time.Time, heartbeatTimeout time.Duration) Evaluation {
	checks := make([]Check, 0, 6)
	blocked := false
	mismatch := false
	stale := false

	if c.TrustState == domain.CompanionTrusted {
		checks = append(checks, Check{Key: "trust", Status: CheckPass, Reason: "Companion identity is trusted"})
	} else {
		blocked = true
		checks = append(checks, Check{Key: "trust", Status: CheckBlock, Reason: fmt.Sprintf("Companion trust state is %s", c.TrustState)})
	}

	if heartbeatTimeout <= 0 {
		blocked = true
		checks = append(checks, Check{Key: "heartbeat", Status: CheckBlock, Reason: "Heartbeat timeout is not configured"})
	} else if c.LastSeenAt.IsZero() || now.UTC().After(c.LastSeenAt.UTC().Add(heartbeatTimeout)) {
		stale = true
		checks = append(checks, Check{Key: "heartbeat", Status: CheckBlock, Reason: "Companion heartbeat is stale"})
	} else {
		checks = append(checks, Check{Key: "heartbeat", Status: CheckPass, Reason: "Companion heartbeat is current"})
	}

	missing := missingCapabilities(role.RequiredCapabilities, c.Capabilities)
	if len(missing) > 0 {
		blocked = true
		checks = append(checks, Check{Key: "capabilities", Status: CheckBlock, Reason: fmt.Sprintf("Missing required capabilities: %v", missing)})
	} else {
		checks = append(checks, Check{Key: "capabilities", Status: CheckPass, Reason: "Required capabilities are available"})
	}

	if role.RequiredRuntimeSnapshotID == nil {
		checks = append(checks, Check{Key: "runtime_snapshot", Status: CheckPass, Reason: "Role has no required Runtime Snapshot"})
	} else if c.AppliedRuntimeSnapshotID == nil || *c.AppliedRuntimeSnapshotID != *role.RequiredRuntimeSnapshotID {
		mismatch = true
		checks = append(checks, Check{Key: "runtime_snapshot", Status: CheckBlock, Reason: "Applied Runtime Snapshot does not match role requirement"})
	} else {
		checks = append(checks, Check{Key: "runtime_snapshot", Status: CheckPass, Reason: "Applied Runtime Snapshot matches"})
	}

	if role.RequiredConfigHash == "" {
		checks = append(checks, Check{Key: "role_config", Status: CheckPass, Reason: "Role has no required configuration hash"})
	} else if c.ConfigHash != role.RequiredConfigHash {
		mismatch = true
		checks = append(checks, Check{Key: "role_config", Status: CheckBlock, Reason: "Applied role configuration does not match"})
	} else {
		checks = append(checks, Check{Key: "role_config", Status: CheckPass, Reason: "Applied role configuration matches"})
	}

	agentCheck := Check{Key: "agent_readiness", Status: CheckWarn, Reason: fmt.Sprintf("Companion reports %s", c.Readiness)}
	switch c.Readiness {
	case domain.CompanionReadinessReady:
		agentCheck.Status = CheckPass
	case domain.CompanionReadinessBlocked:
		agentCheck.Status = CheckBlock
		blocked = true
	case domain.CompanionReadinessMismatch:
		agentCheck.Status = CheckBlock
		mismatch = true
	case domain.CompanionReadinessOffline:
		agentCheck.Status = CheckBlock
		stale = true
	}
	checks = append(checks, agentCheck)

	if stale {
		return Evaluation{Readiness: domain.CompanionReadinessOffline, RoleState: domain.RoleOffline, Checks: checks}
	}
	if mismatch {
		return Evaluation{Readiness: domain.CompanionReadinessMismatch, RoleState: domain.RoleMismatch, Checks: checks}
	}
	if blocked {
		return Evaluation{Readiness: domain.CompanionReadinessBlocked, RoleState: domain.RoleDegraded, Checks: checks}
	}

	switch c.Readiness {
	case domain.CompanionReadinessReady:
		return Evaluation{Readiness: domain.CompanionReadinessReady, RoleState: domain.RoleReady, Checks: checks}
	case domain.CompanionReadinessSyncing:
		return Evaluation{Readiness: domain.CompanionReadinessSyncing, RoleState: domain.RoleSyncing, Checks: checks}
	case domain.CompanionReadinessDegraded:
		return Evaluation{Readiness: domain.CompanionReadinessDegraded, RoleState: domain.RoleDegraded, Checks: checks}
	default:
		return Evaluation{Readiness: domain.CompanionReadinessUnknown, RoleState: domain.RoleAssigned, Checks: checks}
	}
}

func missingCapabilities(required, available []string) []string {
	availableSet := make(map[string]struct{}, len(available))
	for _, capability := range available {
		availableSet[capability] = struct{}{}
	}
	missing := make([]string, 0)
	for _, capability := range required {
		if _, ok := availableSet[capability]; !ok {
			missing = append(missing, capability)
		}
	}
	sort.Strings(missing)
	return missing
}
