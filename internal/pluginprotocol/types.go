package pluginprotocol

import "encoding/json"

const SchemaVersion = 1

type Ready struct {
	Type          string   `json:"type"`
	SchemaVersion int      `json:"schema_version"`
	PluginID      string   `json:"plugin_id"`
	PluginVersion string   `json:"plugin_version"`
	Capabilities  []string `json:"capabilities,omitempty"`
	Inputs        []string `json:"inputs,omitempty"`
	ListenAddress string   `json:"listen_address,omitempty"`
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
	TimeoutMS     int64           `json:"timeout_ms"`
	CorrelationID string          `json:"correlation_id"`
}

type ExecutionResult struct {
	Type          string `json:"type"`
	SchemaVersion int    `json:"schema_version"`
	ExecutionID   string `json:"execution_id"`
	Status        string `json:"status"`
	AckLevel      string `json:"ack_level,omitempty"`
	ErrorCode     string `json:"error_code,omitempty"`
	ErrorCategory string `json:"error_category,omitempty"`
	ErrorMessage  string `json:"error_message,omitempty"`
	DurationMS    int64  `json:"duration_ms"`
}

// InputEvent is an unsolicited, normalized input contribution emitted by an
// external Plugin process. It contains transport-derived data only; Hub Core
// still resolves the active Runtime Snapshot and performs authoritative Route
// evaluation and dispatch.
type InputEvent struct {
	Type          string          `json:"type"`
	SchemaVersion int             `json:"schema_version"`
	InputType     string          `json:"input_type"`
	Source        string          `json:"source"`
	Value         json.RawMessage `json:"value"`
}
