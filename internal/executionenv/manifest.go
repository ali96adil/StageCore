package executionenv

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ali96adil/StageCore/internal/canonicaljson"
)

const ManifestSchemaVersion = 1

const (
	maxKeyBytes        = 96
	maxNameBytes       = 256
	maxConstraintBytes = 128
	maxLocatorBytes    = 2048
	maxAssets          = 256
	maxExtensions      = 128
	maxBindings        = 128
	maxHosts           = 16
)

type CapturePolicy string

const (
	CaptureContentBound  CapturePolicy = "CONTENT_BOUND"
	CaptureReferenceOnly CapturePolicy = "REFERENCE_ONLY"
)

type AssetKind string

const (
	AssetProjectFile AssetKind = "PROJECT_FILE"
	AssetSessionFile AssetKind = "SESSION_FILE"
	AssetMedia       AssetKind = "MEDIA"
	AssetPreset      AssetKind = "PRESET"
	AssetConfig      AssetKind = "CONFIG"
	AssetResource    AssetKind = "RESOURCE"
	AssetOther       AssetKind = "OTHER"
)

type BindingKind string

const (
	BindingInput   BindingKind = "INPUT"
	BindingOutput  BindingKind = "OUTPUT"
	BindingMIDI    BindingKind = "MIDI"
	BindingAudio   BindingKind = "AUDIO"
	BindingDisplay BindingKind = "DISPLAY"
	BindingNetwork BindingKind = "NETWORK"
	BindingOther   BindingKind = "OTHER"
)

type LaunchKind string

const (
	LaunchAsset   LaunchKind = "ASSET"
	LaunchLocator LaunchKind = "LOCATOR"
)

type HostRequirement struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}

type ApplicationRequirement struct {
	Key               string            `json:"key"`
	Name              string            `json:"name"`
	Vendor            string            `json:"vendor,omitempty"`
	VersionConstraint string            `json:"version_constraint"`
	Hosts             []HostRequirement `json:"hosts"`
}

type AssetRequirement struct {
	Key           string        `json:"key"`
	Kind          AssetKind     `json:"kind"`
	Name          string        `json:"name"`
	CapturePolicy CapturePolicy `json:"capture_policy"`
	ContentHash   string        `json:"content_hash,omitempty"`
	SizeBytes     *int64        `json:"size_bytes,omitempty"`
	Locator       string        `json:"locator,omitempty"`
}

type ExternalExtensionRequirement struct {
	Key               string `json:"key"`
	Name              string `json:"name"`
	Vendor            string `json:"vendor,omitempty"`
	VersionConstraint string `json:"version_constraint"`
	Required          bool   `json:"required"`
}

type BindingRequirement struct {
	Key                string      `json:"key"`
	Kind               BindingKind `json:"kind"`
	Name               string      `json:"name"`
	ExternalRef        string      `json:"external_ref"`
	StageCoreTargetRef string      `json:"stagecore_target_ref,omitempty"`
	Required           bool        `json:"required"`
}

type LaunchTarget struct {
	Kind     LaunchKind `json:"kind"`
	AssetKey string     `json:"asset_key,omitempty"`
	Locator  string     `json:"locator,omitempty"`
}

type Manifest struct {
	SchemaVersion  int                            `json:"schema_version"`
	EnvironmentKey string                         `json:"environment_key"`
	Name           string                         `json:"name"`
	AdapterKey     string                         `json:"adapter_key"`
	Application    ApplicationRequirement         `json:"application"`
	Assets         []AssetRequirement             `json:"assets,omitempty"`
	Extensions     []ExternalExtensionRequirement `json:"external_extensions,omitempty"`
	Bindings       []BindingRequirement           `json:"bindings,omitempty"`
	Launch         *LaunchTarget                  `json:"launch,omitempty"`
}

// Normalize validates a manifest and returns a deterministic copy suitable for hashing.
// It never mutates the caller's value.
func Normalize(manifest Manifest) (Manifest, error) {
	normalized := manifest
	normalized.Application.Hosts = append([]HostRequirement(nil), manifest.Application.Hosts...)
	normalized.Assets = append([]AssetRequirement(nil), manifest.Assets...)
	normalized.Extensions = append([]ExternalExtensionRequirement(nil), manifest.Extensions...)
	normalized.Bindings = append([]BindingRequirement(nil), manifest.Bindings...)
	if manifest.Launch != nil {
		launch := *manifest.Launch
		normalized.Launch = &launch
	}

	if normalized.SchemaVersion != ManifestSchemaVersion {
		return Manifest{}, fmt.Errorf("schema_version must be %d", ManifestSchemaVersion)
	}
	if err := validateKey("environment_key", normalized.EnvironmentKey); err != nil {
		return Manifest{}, err
	}
	if err := validateText("name", normalized.Name, maxNameBytes, true); err != nil {
		return Manifest{}, err
	}
	if err := validateKey("adapter_key", normalized.AdapterKey); err != nil {
		return Manifest{}, err
	}
	if err := normalizeApplication(&normalized.Application); err != nil {
		return Manifest{}, err
	}
	if len(normalized.Assets) > maxAssets {
		return Manifest{}, fmt.Errorf("assets exceeds maximum of %d", maxAssets)
	}
	if len(normalized.Extensions) > maxExtensions {
		return Manifest{}, fmt.Errorf("external_extensions exceeds maximum of %d", maxExtensions)
	}
	if len(normalized.Bindings) > maxBindings {
		return Manifest{}, fmt.Errorf("bindings exceeds maximum of %d", maxBindings)
	}

	assetKeys := make(map[string]struct{}, len(normalized.Assets))
	for i := range normalized.Assets {
		asset := &normalized.Assets[i]
		if err := normalizeAsset(asset); err != nil {
			return Manifest{}, fmt.Errorf("assets[%d]: %w", i, err)
		}
		if _, exists := assetKeys[asset.Key]; exists {
			return Manifest{}, fmt.Errorf("duplicate asset key %q", asset.Key)
		}
		assetKeys[asset.Key] = struct{}{}
	}
	if err := validateExtensionRequirements(normalized.Extensions); err != nil {
		return Manifest{}, err
	}
	if err := validateBindings(normalized.Bindings); err != nil {
		return Manifest{}, err
	}
	if normalized.Launch != nil {
		if err := validateLaunch(*normalized.Launch, assetKeys); err != nil {
			return Manifest{}, err
		}
	}

	sort.Slice(normalized.Application.Hosts, func(i, j int) bool {
		if normalized.Application.Hosts[i].OS == normalized.Application.Hosts[j].OS {
			return normalized.Application.Hosts[i].Architecture < normalized.Application.Hosts[j].Architecture
		}
		return normalized.Application.Hosts[i].OS < normalized.Application.Hosts[j].OS
	})
	sort.Slice(normalized.Assets, func(i, j int) bool { return normalized.Assets[i].Key < normalized.Assets[j].Key })
	sort.Slice(normalized.Extensions, func(i, j int) bool { return normalized.Extensions[i].Key < normalized.Extensions[j].Key })
	sort.Slice(normalized.Bindings, func(i, j int) bool { return normalized.Bindings[i].Key < normalized.Bindings[j].Key })

	return normalized, nil
}

func CanonicalBytes(manifest Manifest) ([]byte, error) {
	normalized, err := Normalize(manifest)
	if err != nil {
		return nil, err
	}
	return canonicaljson.Marshal(normalized)
}

func ContentHash(manifest Manifest) (string, error) {
	payload, err := CanonicalBytes(manifest)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func normalizeApplication(application *ApplicationRequirement) error {
	if err := validateKey("application.key", application.Key); err != nil {
		return err
	}
	if err := validateText("application.name", application.Name, maxNameBytes, true); err != nil {
		return err
	}
	if err := validateText("application.vendor", application.Vendor, maxNameBytes, false); err != nil {
		return err
	}
	if err := validateText("application.version_constraint", application.VersionConstraint, maxConstraintBytes, true); err != nil {
		return err
	}
	if len(application.Hosts) == 0 {
		return fmt.Errorf("application.hosts must contain at least one supported host")
	}
	if len(application.Hosts) > maxHosts {
		return fmt.Errorf("application.hosts exceeds maximum of %d", maxHosts)
	}
	seen := make(map[string]struct{}, len(application.Hosts))
	for i := range application.Hosts {
		host := &application.Hosts[i]
		host.OS = strings.ToLower(host.OS)
		host.Architecture = strings.ToLower(host.Architecture)
		if !validHost(host.OS, host.Architecture) {
			return fmt.Errorf("application.hosts[%d]: unsupported host %s/%s", i, host.OS, host.Architecture)
		}
		key := host.OS + "/" + host.Architecture
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate application host %q", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func normalizeAsset(asset *AssetRequirement) error {
	if err := validateKey("asset.key", asset.Key); err != nil {
		return err
	}
	if err := validateText("asset.name", asset.Name, maxNameBytes, true); err != nil {
		return err
	}
	switch asset.Kind {
	case AssetProjectFile, AssetSessionFile, AssetMedia, AssetPreset, AssetConfig, AssetResource, AssetOther:
	default:
		return fmt.Errorf("asset %q has unsupported kind %q", asset.Key, asset.Kind)
	}
	if asset.Locator != "" {
		if err := validateLocator("asset.locator", asset.Locator); err != nil {
			return err
		}
	}
	switch asset.CapturePolicy {
	case CaptureContentBound:
		if !isSHA256(asset.ContentHash) {
			return fmt.Errorf("asset %q content_hash must be a 64-character SHA-256 hex digest", asset.Key)
		}
		asset.ContentHash = strings.ToLower(asset.ContentHash)
		if asset.SizeBytes == nil || *asset.SizeBytes < 0 {
			return fmt.Errorf("asset %q size_bytes is required and must be non-negative for CONTENT_BOUND", asset.Key)
		}
	case CaptureReferenceOnly:
		if asset.ContentHash != "" || asset.SizeBytes != nil {
			return fmt.Errorf("asset %q REFERENCE_ONLY must not claim content_hash or size_bytes", asset.Key)
		}
		if asset.Locator == "" {
			return fmt.Errorf("asset %q REFERENCE_ONLY requires locator", asset.Key)
		}
	default:
		return fmt.Errorf("asset %q has unsupported capture_policy %q", asset.Key, asset.CapturePolicy)
	}
	return nil
}

func validateExtensionRequirements(extensions []ExternalExtensionRequirement) error {
	seen := make(map[string]struct{}, len(extensions))
	for i, extension := range extensions {
		if err := validateKey("external_extension.key", extension.Key); err != nil {
			return fmt.Errorf("external_extensions[%d]: %w", i, err)
		}
		if _, exists := seen[extension.Key]; exists {
			return fmt.Errorf("duplicate external extension key %q", extension.Key)
		}
		seen[extension.Key] = struct{}{}
		if err := validateText("external_extension.name", extension.Name, maxNameBytes, true); err != nil {
			return fmt.Errorf("external_extensions[%d]: %w", i, err)
		}
		if err := validateText("external_extension.vendor", extension.Vendor, maxNameBytes, false); err != nil {
			return fmt.Errorf("external_extensions[%d]: %w", i, err)
		}
		if err := validateText("external_extension.version_constraint", extension.VersionConstraint, maxConstraintBytes, true); err != nil {
			return fmt.Errorf("external_extensions[%d]: %w", i, err)
		}
	}
	return nil
}

func validateBindings(bindings []BindingRequirement) error {
	seen := make(map[string]struct{}, len(bindings))
	for i, binding := range bindings {
		if err := validateKey("binding.key", binding.Key); err != nil {
			return fmt.Errorf("bindings[%d]: %w", i, err)
		}
		if _, exists := seen[binding.Key]; exists {
			return fmt.Errorf("duplicate binding key %q", binding.Key)
		}
		seen[binding.Key] = struct{}{}
		if err := validateText("binding.name", binding.Name, maxNameBytes, true); err != nil {
			return fmt.Errorf("bindings[%d]: %w", i, err)
		}
		switch binding.Kind {
		case BindingInput, BindingOutput, BindingMIDI, BindingAudio, BindingDisplay, BindingNetwork, BindingOther:
		default:
			return fmt.Errorf("binding %q has unsupported kind %q", binding.Key, binding.Kind)
		}
		if err := validateLocator("binding.external_ref", binding.ExternalRef); err != nil {
			return fmt.Errorf("bindings[%d]: %w", i, err)
		}
		if binding.StageCoreTargetRef != "" {
			if err := validateText("binding.stagecore_target_ref", binding.StageCoreTargetRef, maxNameBytes, true); err != nil {
				return fmt.Errorf("bindings[%d]: %w", i, err)
			}
		}
	}
	return nil
}

func validateLaunch(launch LaunchTarget, assetKeys map[string]struct{}) error {
	switch launch.Kind {
	case LaunchAsset:
		if err := validateKey("launch.asset_key", launch.AssetKey); err != nil {
			return err
		}
		if launch.Locator != "" {
			return fmt.Errorf("launch ASSET must not include locator")
		}
		if _, exists := assetKeys[launch.AssetKey]; !exists {
			return fmt.Errorf("launch references unknown asset %q", launch.AssetKey)
		}
	case LaunchLocator:
		if launch.AssetKey != "" {
			return fmt.Errorf("launch LOCATOR must not include asset_key")
		}
		if err := validateLocator("launch.locator", launch.Locator); err != nil {
			return err
		}
	default:
		return fmt.Errorf("launch has unsupported kind %q", launch.Kind)
	}
	return nil
}

func validateKey(label, value string) error {
	if value == "" || len(value) > maxKeyBytes {
		return fmt.Errorf("%s must be 1..%d bytes", label, maxKeyBytes)
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_' {
			continue
		}
		return fmt.Errorf("%s contains invalid character %q", label, c)
	}
	if value[0] == '.' || value[0] == '-' || value[0] == '_' {
		return fmt.Errorf("%s must start with a lowercase letter or digit", label)
	}
	return nil
}

func validateText(label, value string, maxBytes int, required bool) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must be valid UTF-8", label)
	}
	if required && value == "" {
		return fmt.Errorf("%s is required", label)
	}
	if len(value) > maxBytes {
		return fmt.Errorf("%s exceeds maximum of %d bytes", label, maxBytes)
	}
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("%s must not have leading or trailing whitespace", label)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("%s contains control characters", label)
		}
	}
	return nil
}

func validateLocator(label, value string) error {
	if err := validateText(label, value, maxLocatorBytes, true); err != nil {
		return err
	}
	pathLike := strings.ReplaceAll(value, "\\", "/")
	for _, segment := range strings.Split(pathLike, "/") {
		if segment == ".." {
			return fmt.Errorf("%s must not contain parent traversal segments", label)
		}
	}
	return nil
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validHost(osName, architecture string) bool {
	switch osName {
	case "darwin", "linux", "windows":
	default:
		return false
	}
	switch architecture {
	case "arm64", "amd64":
		return true
	default:
		return false
	}
}
