package protocol

import "encoding/json"

const SchemaVersion = 1

type Ready struct {
	Type          string   `json:"type"`
	SchemaVersion int      `json:"schema_version"`
	PluginID      string   `json:"plugin_id"`
	PluginVersion string   `json:"plugin_version"`
	Capabilities  []string `json:"capabilities"`
}

type ResolvedTarget struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type ExecutionRequest struct {
	Type          string          `json:"type"`
	SchemaVersion int             `json:"schema_version"`
	ExecutionID   string          `json:"execution_id"`
	Capability    string          `json:"capability"`
	Target        ResolvedTarget  `json:"target"`
	Parameters    json.RawMessage `json:"parameters"`
	Priority      string          `json:"priority"`
	TimeoutMS     int             `json:"timeout_ms"`
	CorrelationID string          `json:"correlation_id"`
}

type ExecutionResult struct {
	Type          string `json:"type"`
	SchemaVersion int    `json:"schema_version"`
	ExecutionID   string `json:"execution_id"`
	Status        string `json:"status"`
	AckLevel      string `json:"ack_level,omitempty"`
	ErrorCategory string `json:"error_category,omitempty"`
	ErrorMessage  string `json:"error_message,omitempty"`
	DurationMS    int64  `json:"duration_ms"`
}
