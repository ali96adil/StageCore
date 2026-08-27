package domain

import "time"

type CompanionTrustState string

const (
	CompanionUntrusted CompanionTrustState = "UNTRUSTED"
	CompanionTrusted   CompanionTrustState = "TRUSTED"
	CompanionRevoked   CompanionTrustState = "REVOKED"
)

type CompanionReadiness string

const (
	CompanionReadinessUnknown  CompanionReadiness = "UNKNOWN"
	CompanionReadinessSyncing  CompanionReadiness = "SYNCING"
	CompanionReadinessReady    CompanionReadiness = "READY"
	CompanionReadinessDegraded CompanionReadiness = "DEGRADED"
	CompanionReadinessOffline  CompanionReadiness = "OFFLINE"
	CompanionReadinessMismatch CompanionReadiness = "MISMATCH"
	CompanionReadinessBlocked  CompanionReadiness = "BLOCKED"
)

type RoleAssignmentState string

const (
	RoleUnassigned RoleAssignmentState = "UNASSIGNED"
	RoleAssigned   RoleAssignmentState = "ASSIGNED"
	RoleSyncing    RoleAssignmentState = "SYNCING"
	RoleReady      RoleAssignmentState = "READY"
	RoleDegraded   RoleAssignmentState = "DEGRADED"
	RoleOffline    RoleAssignmentState = "OFFLINE"
	RoleMismatch   RoleAssignmentState = "MISMATCH"
	RoleReleased   RoleAssignmentState = "RELEASED"
)

type Companion struct {
	ID                       string
	DisplayName              string
	Hostname                 string
	Platform                 string
	Architecture             string
	Version                  string
	Capabilities             []string
	LastSeenAt               time.Time
	TrustState               CompanionTrustState
	Readiness                CompanionReadiness
	AppliedRuntimeSnapshotID *string
	ConfigHash               string
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

type MachineRole struct {
	ID                        string
	ProjectID                 string
	RoleKey                   string
	DisplayName               string
	RequiredCapabilities      []string
	RequiredRuntimeSnapshotID *string
	RequiredConfigHash        string
	Required                  bool
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

type RoleAssignment struct {
	ID              string
	MachineRoleID   string
	CompanionID     string
	State           RoleAssignmentState
	AssignedAt      time.Time
	ReleasedAt      *time.Time
	LastEvaluatedAt time.Time
}
