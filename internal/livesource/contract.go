package livesource

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

const (
	LogicalType      = "live_video_source"
	ContractVersion1 = 1

	CapabilityOpen    = "video.source.open"
	CapabilityClose   = "video.source.close"
	CapabilitySelect  = "video.source.select"
	CapabilityRoute   = "video.source.route"
	CapabilityInspect = "video.source.inspect"

	DefaultObservationMaxAge = 5 * time.Second
	MaxObservationMaxAge     = 30 * time.Second
)

type SourceClass string

const (
	SourceLocalCamera   SourceClass = "LOCAL_CAMERA"
	SourceUSBCapture    SourceClass = "USB_CAPTURE"
	SourceNetworkStream SourceClass = "NETWORK_STREAM"
)

type Readiness string

const (
	ReadinessReady    Readiness = "READY"
	ReadinessWarning  Readiness = "WARNING"
	ReadinessAdvisory Readiness = "ADVISORY"
	ReadinessBlocker  Readiness = "BLOCKER"
)

type TargetConfig struct {
	ContractVersion     int             `json:"contract_version"`
	SourceID            string          `json:"source_id"`
	Name                string          `json:"name"`
	SourceClass         SourceClass     `json:"source_class"`
	MachineRoleID       string          `json:"machine_role_id"`
	DeviceProfileRef    string          `json:"device_profile_ref,omitempty"`
	AdapterConfig       json.RawMessage `json:"adapter_config,omitempty"`
	Required            bool            `json:"required"`
	DesiredEnabled      bool            `json:"desired_enabled"`
	RequiredCapabilities []string       `json:"required_capabilities,omitempty"`
	ObservationMaxAgeMS int64           `json:"observation_max_age_ms,omitempty"`
}

type Command struct {
	ContractVersion  int             `json:"contract_version"`
	SourceID         string          `json:"source_id"`
	SourceClass      SourceClass     `json:"source_class"`
	DeviceProfileRef string          `json:"device_profile_ref,omitempty"`
	AdapterConfig    json.RawMessage `json:"adapter_config"`
	Parameters       json.RawMessage `json:"parameters"`
}

type Observation struct {
	ContractVersion int         `json:"contract_version"`
	SourceID        string      `json:"source_id"`
	Readiness       Readiness   `json:"readiness"`
	ObservedAt      time.Time   `json:"observed_at"`
	Capabilities    []string    `json:"capabilities,omitempty"`
	Summary         string      `json:"summary,omitempty"`
	Generation      uint64      `json:"generation,omitempty"`
}

var allCapabilities = map[string]struct{}{
	CapabilityOpen: {}, CapabilityClose: {}, CapabilitySelect: {}, CapabilityRoute: {}, CapabilityInspect: {},
}

func SupportsCapability(value string) bool {
	_, ok := allCapabilities[strings.TrimSpace(value)]
	return ok
}

func DecodeTargetConfig(raw json.RawMessage) (TargetConfig, error) {
	var cfg TargetConfig
	if err := strictDecode(raw, &cfg); err != nil {
		return TargetConfig{}, fmt.Errorf("live source target config: %w", err)
	}
	cfg.SourceID = strings.TrimSpace(cfg.SourceID)
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.MachineRoleID = strings.TrimSpace(cfg.MachineRoleID)
	cfg.DeviceProfileRef = strings.TrimSpace(cfg.DeviceProfileRef)
	if cfg.ContractVersion != ContractVersion1 {
		return TargetConfig{}, fmt.Errorf("unsupported live source contract_version %d", cfg.ContractVersion)
	}
	if cfg.SourceID == "" || cfg.Name == "" || cfg.MachineRoleID == "" {
		return TargetConfig{}, errors.New("source_id, name and machine_role_id are required")
	}
	switch cfg.SourceClass {
	case SourceLocalCamera, SourceUSBCapture, SourceNetworkStream:
	default:
		return TargetConfig{}, fmt.Errorf("unsupported source_class %q", cfg.SourceClass)
	}
	if len(cfg.AdapterConfig) == 0 {
		cfg.AdapterConfig = json.RawMessage(`{}`)
	}
	if err := requireJSONObject(cfg.AdapterConfig); err != nil {
		return TargetConfig{}, fmt.Errorf("adapter_config: %w", err)
	}
	seen := map[string]struct{}{}
	for i, capability := range cfg.RequiredCapabilities {
		capability = strings.TrimSpace(capability)
		if !SupportsCapability(capability) {
			return TargetConfig{}, fmt.Errorf("required_capabilities contains unsupported capability %q", capability)
		}
		if _, exists := seen[capability]; exists {
			return TargetConfig{}, fmt.Errorf("required_capabilities contains duplicate capability %q", capability)
		}
		seen[capability] = struct{}{}
		cfg.RequiredCapabilities[i] = capability
	}
	if len(cfg.RequiredCapabilities) == 0 {
		cfg.RequiredCapabilities = []string{CapabilityInspect}
		if cfg.DesiredEnabled {
			cfg.RequiredCapabilities = append(cfg.RequiredCapabilities, CapabilityOpen)
		}
	}
	sort.Strings(cfg.RequiredCapabilities)
	if cfg.ObservationMaxAgeMS < 0 || time.Duration(cfg.ObservationMaxAgeMS)*time.Millisecond > MaxObservationMaxAge {
		return TargetConfig{}, fmt.Errorf("observation_max_age_ms must be between 0 and %d", MaxObservationMaxAge.Milliseconds())
	}
	return cfg, nil
}

func (c TargetConfig) ObservationMaxAge() time.Duration {
	if c.ObservationMaxAgeMS == 0 {
		return DefaultObservationMaxAge
	}
	return time.Duration(c.ObservationMaxAgeMS) * time.Millisecond
}

func EncodeCommand(cfg TargetConfig, parameters json.RawMessage) (json.RawMessage, error) {
	if len(parameters) == 0 {
		parameters = json.RawMessage(`{}`)
	}
	if !json.Valid(parameters) {
		return nil, errors.New("live source command parameters must be valid JSON")
	}
	return json.Marshal(Command{
		ContractVersion: ContractVersion1,
		SourceID: cfg.SourceID,
		SourceClass: cfg.SourceClass,
		DeviceProfileRef: cfg.DeviceProfileRef,
		AdapterConfig: cfg.AdapterConfig,
		Parameters: parameters,
	})
}

func DecodeObservation(raw string) (Observation, error) {
	var observation Observation
	if err := strictDecode(json.RawMessage(strings.TrimSpace(raw)), &observation); err != nil {
		return Observation{}, fmt.Errorf("live source observation: %w", err)
	}
	observation.SourceID = strings.TrimSpace(observation.SourceID)
	observation.Summary = strings.TrimSpace(observation.Summary)
	if observation.ContractVersion != ContractVersion1 || observation.SourceID == "" || observation.ObservedAt.IsZero() {
		return Observation{}, errors.New("live source observation requires contract_version=1, source_id and observed_at")
	}
	switch observation.Readiness {
	case ReadinessReady, ReadinessWarning, ReadinessAdvisory, ReadinessBlocker:
	default:
		return Observation{}, fmt.Errorf("invalid live source readiness %q", observation.Readiness)
	}
	seen := map[string]struct{}{}
	for i, capability := range observation.Capabilities {
		capability = strings.TrimSpace(capability)
		if !SupportsCapability(capability) {
			return Observation{}, fmt.Errorf("observation advertises unsupported capability %q", capability)
		}
		if _, duplicate := seen[capability]; duplicate {
			return Observation{}, fmt.Errorf("observation contains duplicate capability %q", capability)
		}
		seen[capability] = struct{}{}
		observation.Capabilities[i] = capability
	}
	sort.Strings(observation.Capabilities)
	return observation, nil
}

func HasCapability(values []string, wanted string) bool {
	wanted = strings.TrimSpace(wanted)
	for _, value := range values {
		if strings.TrimSpace(value) == wanted {
			return true
		}
	}
	return false
}

func strictDecode(raw json.RawMessage, dst any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return errors.New("JSON object is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func requireJSONObject(raw json.RawMessage) error {
	if !json.Valid(raw) {
		return errors.New("must be valid JSON")
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	if _, ok := value.(map[string]any); !ok {
		return errors.New("must be a JSON object")
	}
	return nil
}
