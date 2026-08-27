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

type SessionStatus string

const (
	SessionActive    SessionStatus = "ACTIVE"
	SessionCompleted SessionStatus = "COMPLETED"
	SessionAborted   SessionStatus = "ABORTED"
)

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
