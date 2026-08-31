package extension

import "time"

const ManifestSchemaVersion = 1

const (
	RuntimeProtocolPluginV1        = "stagecore.plugin.v1"
	RuntimeArtifactNativeExecutable = "native-executable"
)

type Kind string

const (
	KindPlugin Kind = "PLUGIN"
	KindAddon  Kind = "ADDON"
)

type Source string

const (
	SourceOfficial  Source = "OFFICIAL"
	SourceLocal     Source = "LOCAL"
	SourceCommunity Source = "COMMUNITY"
)

type LocalizedText struct {
	EN   string `json:"en"`
	ArIQ string `json:"ar-IQ"`
}

type Compatibility struct {
	APIMin        int      `json:"api_min"`
	APIMax        int      `json:"api_max"`
	Platforms     []string `json:"platforms"`
	Architectures []string `json:"architectures"`
}

type Dependency struct {
	ExtensionID string `json:"extension_id"`
	MinVersion  string `json:"min_version,omitempty"`
	MaxVersion  string `json:"max_version,omitempty"`
	Optional    bool   `json:"optional,omitempty"`
}

type RuntimeContract struct {
	Protocol              string              `json:"protocol"`
	Artifact              string              `json:"artifact"`
	CapabilityPermissions map[string][]string `json:"capability_permissions"`
}

type Manifest struct {
	SchemaVersion int              `json:"schema_version"`
	ExtensionID   string           `json:"extension_id"`
	Version       string           `json:"version"`
	Kind          Kind             `json:"kind"`
	Source        Source           `json:"source"`
	Name          LocalizedText    `json:"name"`
	Summary       LocalizedText    `json:"summary"`
	Compatibility Compatibility    `json:"compatibility"`
	Permissions   []string         `json:"permissions,omitempty"`
	Capabilities  []string         `json:"capabilities,omitempty"`
	Dependencies  []Dependency     `json:"dependencies,omitempty"`
	Runtime       *RuntimeContract `json:"runtime,omitempty"`
}

type Package struct {
	PackageID           string    `json:"package_id"`
	Manifest            Manifest  `json:"manifest"`
	ManifestSHA256      string    `json:"manifest_sha256"`
	Compatible          bool      `json:"compatible"`
	ProductionReady     bool      `json:"production_ready"`
	CompatibilityReason string    `json:"compatibility_reason"`
	RegisteredBy        string    `json:"registered_by"`
	RegisteredAt        time.Time `json:"registered_at"`
}
