package showtemplate

import "encoding/json"

const (
	SchemaVersion      = 1
	CurrentAPIVersion  = 1
)

type Source string

const (
	SourceOfficial Source = "OFFICIAL"
	SourceImported Source = "IMPORTED"
	SourceExported Source = "EXPORTED"
)

type LocalizedText struct {
	EN   string `json:"en"`
	ArIQ string `json:"ar-IQ"`
}

type FieldType string

const (
	FieldString FieldType = "STRING"
	FieldInt    FieldType = "INT"
	FieldBool   FieldType = "BOOL"
)

type Field struct {
	Key          string          `json:"key"`
	Type         FieldType       `json:"type"`
	Required     bool            `json:"required"`
	Label        LocalizedText   `json:"label"`
	Help         LocalizedText   `json:"help"`
	DefaultValue json.RawMessage `json:"default_value,omitempty"`
	MinInt       *int64          `json:"min_int,omitempty"`
	MaxInt       *int64          `json:"max_int,omitempty"`
	MaxLength    int             `json:"max_length,omitempty"`
}

type TargetSpec struct {
	Key           string          `json:"key"`
	LogicalName   string          `json:"logical_name"`
	LogicalType   string          `json:"logical_type"`
	TargetRef     string          `json:"target_ref,omitempty"`
	GroupName     string          `json:"group_name,omitempty"`
	Configuration json.RawMessage `json:"configuration"`
}

type ActionSpec struct {
	Key            string          `json:"key"`
	OrderIndex     int             `json:"order_index"`
	ExecutionMode  string          `json:"execution_mode"`
	TargetKey      string          `json:"target_key"`
	CapabilityKey  string          `json:"capability_key"`
	Parameters     json.RawMessage `json:"parameters"`
	TimeoutPolicy  json.RawMessage `json:"timeout_policy"`
	ErrorPolicy    json.RawMessage `json:"error_policy"`
	PriorityClass  string          `json:"priority_class"`
	Enabled        bool            `json:"enabled"`
}

type CueSpec struct {
	Key              string          `json:"key"`
	DisplayLabel     string          `json:"display_label"`
	Name             LocalizedText   `json:"name"`
	OrderIndex       int             `json:"order_index"`
	CueType          string          `json:"cue_type"`
	Criticality      string          `json:"criticality"`
	Enabled          bool            `json:"enabled"`
	ExecutionPolicy  json.RawMessage `json:"execution_policy"`
	Notes            LocalizedText   `json:"notes"`
	Actions          []ActionSpec    `json:"actions,omitempty"`
}

type InputSpec struct {
	Key         string          `json:"key"`
	Name        LocalizedText   `json:"name"`
	SourceRef   string          `json:"source_ref"`
	EventType   string          `json:"event_type"`
	ValueSchema json.RawMessage `json:"value_schema"`
	Enabled     bool            `json:"enabled"`
}

type OutputSpec struct {
	Key           string          `json:"key"`
	Name          LocalizedText   `json:"name"`
	TargetKey     string          `json:"target_key"`
	CapabilityKey string          `json:"capability_key"`
	ValueSchema   json.RawMessage `json:"value_schema"`
	Criticality   string          `json:"criticality"`
}

type RouteActionSpec struct {
	Key         string          `json:"key"`
	OrderIndex  int             `json:"order_index"`
	OutputKey   string          `json:"output_key,omitempty"`
	CueKey      string          `json:"cue_key,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type RouteSpec struct {
	Key                  string            `json:"key"`
	Name                 LocalizedText     `json:"name"`
	InputKey             string            `json:"input_key"`
	ConditionDefinition  json.RawMessage   `json:"condition_definition"`
	TransformDefinition  json.RawMessage   `json:"transform_definition"`
	DelayMS              *int64            `json:"delay_ms,omitempty"`
	DebounceMS           *int64            `json:"debounce_ms,omitempty"`
	PriorityClass        string            `json:"priority_class"`
	ErrorPolicy          json.RawMessage   `json:"error_policy"`
	Enabled              bool              `json:"enabled"`
	Actions              []RouteActionSpec `json:"actions"`
}

type ProjectSpec struct {
	DefaultName        LocalizedText `json:"default_name"`
	DefaultDescription LocalizedText `json:"default_description"`
	Targets            []TargetSpec  `json:"targets,omitempty"`
	Cues               []CueSpec     `json:"cues,omitempty"`
	Inputs             []InputSpec   `json:"inputs,omitempty"`
	Outputs            []OutputSpec  `json:"outputs,omitempty"`
	Routes             []RouteSpec   `json:"routes,omitempty"`
}

type Template struct {
	SchemaVersion int           `json:"schema_version"`
	MinAPIVersion int           `json:"min_api_version"`
	MaxAPIVersion int           `json:"max_api_version"`
	ID            string        `json:"template_id"`
	Version       string        `json:"version"`
	Source        Source        `json:"source"`
	Name          LocalizedText `json:"name"`
	Summary       LocalizedText `json:"summary"`
	Tags          []string      `json:"tags,omitempty"`
	Fields        []Field       `json:"fields,omitempty"`
	Project       ProjectSpec   `json:"project"`
}

type Compatibility struct {
	Compatible bool     `json:"compatible"`
	Reasons    []string `json:"reasons,omitempty"`
}

type MaterializeRequest struct {
	ProjectName        string         `json:"project_name"`
	ProjectDescription string         `json:"project_description"`
	Locale             string         `json:"locale"`
	Values             map[string]any `json:"values,omitempty"`
	CreatedBy          string         `json:"-"`
}

type MaterializeResult struct {
	TemplateID string `json:"template_id"`
	ProjectID  string `json:"project_id"`
	RevisionID string `json:"revision_id"`
}
