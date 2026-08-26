package contracts

import (
	"encoding/json"
	"time"
)

const SchemaVersion1 = 1

type CommandStatus string

const (
	CommandAccepted  CommandStatus = "ACCEPTED"
	CommandRejected  CommandStatus = "REJECTED"
	CommandCompleted CommandStatus = "COMPLETED"
	CommandFailed    CommandStatus = "FAILED"
	CommandTimedOut  CommandStatus = "TIMED_OUT"
	CommandCancelled CommandStatus = "CANCELLED"
)

type AckLevel string

const (
	AckNone          AckLevel = "NONE"
	AckTransportOnly AckLevel = "TRANSPORT_ONLY"
	AckAccepted      AckLevel = "ACCEPTED"
	AckDevice        AckLevel = "DEVICE_ACK"
	AckVerifiedState AckLevel = "VERIFIED_STATE"
)

type CommandEnvelope struct {
	CommandID         string          `json:"command_id"`
	CommandType       string          `json:"command_type"`
	SchemaVersion     int             `json:"schema_version"`
	IssuedAt          time.Time       `json:"issued_at"`
	DeadlineAt        *time.Time      `json:"deadline_at,omitempty"`
	ProjectID         string          `json:"project_id"`
	RuntimeSnapshotID string          `json:"runtime_snapshot_id,omitempty"`
	Issuer            string          `json:"issuer"`
	CorrelationID     string          `json:"correlation_id,omitempty"`
	CausationID       string          `json:"causation_id,omitempty"`
	Priority          string          `json:"priority"`
	IdempotencyKey    string          `json:"idempotency_key,omitempty"`
	Payload           json.RawMessage `json:"payload"`
}

type ContractError struct {
	ErrorCode        string          `json:"error_code"`
	Category         string          `json:"category"`
	Message          string          `json:"message"`
	Retryable        bool            `json:"retryable"`
	AffectedEntityID string          `json:"affected_entity_id,omitempty"`
	Details          json.RawMessage `json:"details,omitempty"`
	OperatorAction   *string         `json:"operator_action,omitempty"`
}

type CommandResult struct {
	CommandID string          `json:"command_id"`
	Status    CommandStatus   `json:"status"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Error     *ContractError  `json:"error,omitempty"`
}

type EventEnvelope struct {
	EventID           string          `json:"event_id"`
	EventType         string          `json:"event_type"`
	SchemaVersion     int             `json:"schema_version"`
	OccurredAt        time.Time       `json:"occurred_at"`
	ObservedAt        time.Time       `json:"observed_at"`
	Source            string          `json:"source"`
	ProjectID         string          `json:"project_id"`
	RuntimeSnapshotID string          `json:"runtime_snapshot_id,omitempty"`
	CorrelationID     string          `json:"correlation_id,omitempty"`
	CausationID       string          `json:"causation_id,omitempty"`
	Priority          string          `json:"priority"`
	Sequence          int64           `json:"sequence"`
	TraceContext      json.RawMessage `json:"trace_context,omitempty"`
	Payload           json.RawMessage `json:"payload"`
}
