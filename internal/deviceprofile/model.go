package deviceprofile

import "encoding/json"

const CatalogSchemaVersion = 1

type Source string

const (
	SourceOfficial  Source = "OFFICIAL"
	SourceLocal     Source = "LOCAL"
	SourceCommunity Source = "COMMUNITY"
)

type Kind string

const (
	KindDevice   Kind = "DEVICE"
	KindSoftware Kind = "SOFTWARE"
	KindService  Kind = "SERVICE"
)

type FieldType string

const (
	FieldString FieldType = "STRING"
	FieldInt    FieldType = "INT"
	FieldBool   FieldType = "BOOL"
	FieldSecret FieldType = "SECRET"
)

type FieldFormat string

const (
	FormatText FieldFormat = "TEXT"
	FormatHost FieldFormat = "HOST"
	FormatURL  FieldFormat = "URL"
	FormatPath FieldFormat = "PATH"
)

type MatchMode string

const (
	MatchExact    MatchMode = "EXACT"
	MatchPrefix   MatchMode = "PREFIX"
	MatchContains MatchMode = "CONTAINS"
)

type LocalizedText struct {
	EN   string `json:"en"`
	ArIQ string `json:"ar-IQ"`
}

type DiscoveryHint struct {
	Attribute string    `json:"attribute"`
	Mode      MatchMode `json:"mode"`
	Value     string    `json:"value"`
	Weight    int       `json:"weight"`
	Required  bool      `json:"required"`
}

type ConnectionField struct {
	Key          string          `json:"key"`
	Type         FieldType       `json:"type"`
	Format       FieldFormat     `json:"format,omitempty"`
	Required     bool            `json:"required"`
	Label        LocalizedText   `json:"label"`
	Help         LocalizedText   `json:"help"`
	DefaultValue json.RawMessage `json:"default_value,omitempty"`
	MinInt       *int64          `json:"min_int,omitempty"`
	MaxInt       *int64          `json:"max_int,omitempty"`
}

type Action struct {
	ID              string          `json:"action_id"`
	Name            LocalizedText   `json:"name"`
	ParameterSchema json.RawMessage `json:"parameter_schema,omitempty"`
}

type Capability struct {
	Key     string        `json:"capability_key"`
	Name    LocalizedText `json:"name"`
	Actions []Action      `json:"actions,omitempty"`
}

type HealthCheck struct {
	ID        string        `json:"check_id"`
	Type      string        `json:"type"`
	Name      LocalizedText `json:"name"`
	TimeoutMS int           `json:"timeout_ms,omitempty"`
	Field     string        `json:"field,omitempty"`
	Path      string        `json:"path,omitempty"`
}

type TargetTemplate struct {
	LogicalType   string          `json:"logical_type"`
	Configuration json.RawMessage `json:"configuration"`
}

type Profile struct {
	ID                     string            `json:"profile_id"`
	Version                string            `json:"version"`
	Source                 Source            `json:"source"`
	Kind                   Kind              `json:"kind"`
	Name                   LocalizedText     `json:"name"`
	Summary                LocalizedText     `json:"summary"`
	DiscoveryHints         []DiscoveryHint   `json:"discovery_hints,omitempty"`
	ConnectionFields       []ConnectionField `json:"connection_fields,omitempty"`
	Capabilities           []Capability      `json:"capabilities,omitempty"`
	HealthChecks           []HealthCheck     `json:"health_checks,omitempty"`
	TestedProtocolVersions []string          `json:"tested_protocol_versions,omitempty"`
	Tags                   []string          `json:"tags,omitempty"`
	Target                 *TargetTemplate   `json:"target,omitempty"`
}

type Observation struct {
	Attributes map[string]string `json:"attributes"`
}

type MatchCandidate struct {
	ProfileID string   `json:"profile_id"`
	Version   string   `json:"version"`
	Score     int      `json:"score"`
	Reasons   []string `json:"reasons"`
}

type MaterializedTarget struct {
	ProfileID     string          `json:"profile_id"`
	ProfileVersion string         `json:"profile_version"`
	LogicalType   string          `json:"logical_type"`
	Configuration json.RawMessage `json:"configuration"`
}
