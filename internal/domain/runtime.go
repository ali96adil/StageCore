package domain

import (
	"encoding/json"
	"time"
)

type RuntimeSnapshotStatus string

const (
	SnapshotPublished  RuntimeSnapshotStatus = "PUBLISHED"
	SnapshotSuperseded RuntimeSnapshotStatus = "SUPERSEDED"
	SnapshotRevoked    RuntimeSnapshotStatus = "REVOKED"
)

type RuntimeSnapshot struct {
	ID              string
	ProjectID       string
	RevisionID      string
	SnapshotVersion int64
	CreatedAt       time.Time
	CreatedBy       string
	ContentHash     string
	Manifest        json.RawMessage
	Status          RuntimeSnapshotStatus
}

type SessionType string

const (
	SessionRehearsal  SessionType = "REHEARSAL"
	SessionShow       SessionType = "SHOW"
	SessionSimulation SessionType = "SIMULATION"
)

const SessionContractVersion1 = 1

type SessionStatus string

const (
	// SessionStatus is the M1 coarse compatibility projection used by existing
	// runtime gates. LifecycleState is the F-027 authoritative lifecycle.
	SessionActive    SessionStatus = "ACTIVE"
	SessionCompleted SessionStatus = "COMPLETED"
	SessionAborted   SessionStatus = "ABORTED"
)

type SessionLifecycleState string

const (
	SessionLifecycleActive    SessionLifecycleState = "ACTIVE"
	SessionLifecycleCompleted SessionLifecycleState = "COMPLETED"
	SessionLifecycleStopped   SessionLifecycleState = "STOPPED"
	SessionLifecycleSuspended SessionLifecycleState = "SUSPENDED"
	SessionLifecycleAborted   SessionLifecycleState = "ABORTED"
)

type SessionStartPositionKind string

const (
	SessionStartUnspecified SessionStartPositionKind = "UNSPECIFIED"
	SessionStartBeginning   SessionStartPositionKind = "BEGINNING"
	SessionStartCue         SessionStartPositionKind = "CUE"
	SessionStartScene       SessionStartPositionKind = "SCENE"
	SessionStartRange       SessionStartPositionKind = "RANGE"
	SessionStartCheckpoint  SessionStartPositionKind = "CHECKPOINT"
)

type SessionRestorationStatus string

const (
	SessionRestorationNotAssessed                SessionRestorationStatus = "NOT_ASSESSED"
	SessionRestorationNotRequired                SessionRestorationStatus = "NOT_REQUIRED"
	SessionRestorationRestorable                 SessionRestorationStatus = "RESTORABLE"
	SessionRestorationManualConfirmationRequired SessionRestorationStatus = "MANUAL_CONFIRMATION_REQUIRED"
	SessionRestorationUnavailable                SessionRestorationStatus = "UNAVAILABLE"
)

type SessionStartPosition struct {
	Version  int
	Kind     SessionStartPositionKind
	CueID    *string
	Metadata json.RawMessage
}

type SessionStateTruth struct {
	Version                    int
	RestorationStatus          SessionRestorationStatus
	DesiredStateRef            *string
	VerifiedStateRef           *string
	ManualConfirmationRequired bool
}

type Session struct {
	ID                string
	ProjectID         string
	RuntimeSnapshotID string
	Type              SessionType
	Name              string
	StartedAt         time.Time
	EndedAt           *time.Time
	Status            SessionStatus
	CurrentCueID      *string

	ContractVersion    int
	LifecycleState     SessionLifecycleState
	EndReason          string
	StartPosition      SessionStartPosition
	LastCompletedCueID *string
	NextCueID          *string
	StateTruth         SessionStateTruth
}

type ExecutionResult string

const (
	ExecutionRunning   ExecutionResult = "RUNNING"
	ExecutionCompleted ExecutionResult = "COMPLETED"
	ExecutionFailed    ExecutionResult = "FAILED"
	ExecutionTimedOut  ExecutionResult = "TIMED_OUT"
	ExecutionCancelled ExecutionResult = "CANCELLED"
)

type CueExecution struct {
	ID             string
	SessionID      string
	CueID          string
	CorrelationID  string
	TriggerSource  string
	StartedAt      time.Time
	CompletedAt    *time.Time
	Result         ExecutionResult
	ManualOverride bool
}

type ActionExecution struct {
	ID              string
	CueExecutionID  string
	ActionID        string
	StartedAt       time.Time
	CompletedAt     *time.Time
	Result          ExecutionResult
	LatencyMS       *int64
	ResponseSummary string
	ErrorCode       *string
}

type CommandRecord struct {
	CommandID      string
	CommandType    string
	SchemaVersion  int
	ProjectID      string
	SnapshotID     string
	IdempotencyKey string
	CorrelationID  string
	Status         string
	ResultJSON     json.RawMessage
	IssuedAt       time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
