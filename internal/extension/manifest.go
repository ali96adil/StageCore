package extension

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/ali96adil/StageCore/internal/canonicaljson"
)

var (
	extensionIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)+$`)
	versionPattern     = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)
	keyPattern         = regexp.MustCompile(`^[a-z][a-z0-9_-]*(?:\.[a-z0-9_-]+)+$`)
	tokenPattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
)

var knownPermissions = map[string]struct{}{
	"network.udp.send":   {},
	"network.udp.listen": {},
}

func ParseManifest(raw []byte) (Manifest, []byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, nil, fmt.Errorf("decode extension manifest: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Manifest{}, nil, fmt.Errorf("extension manifest contains multiple JSON values")
		}
		return Manifest{}, nil, fmt.Errorf("decode extension manifest trailing data: %w", err)
	}
	manifest = normalizeManifest(manifest)
	if err := ValidateManifest(manifest); err != nil {
		return Manifest{}, nil, err
	}
	canonical, err := canonicaljson.Marshal(manifest)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("canonicalize extension manifest: %w", err)
	}
	return manifest, canonical, nil
}

func ValidateManifest(manifest Manifest) error {
	manifest = normalizeManifest(manifest)
	if manifest.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("unsupported extension manifest schema_version %d", manifest.SchemaVersion)
	}
	if !extensionIDPattern.MatchString(manifest.ExtensionID) {
		return fmt.Errorf("invalid extension_id %q", manifest.ExtensionID)
	}
	if !versionPattern.MatchString(manifest.Version) {
		return fmt.Errorf("extension version must use semantic x.y.z form")
	}
	switch manifest.Kind {
	case KindPlugin, KindAddon:
	default:
		return fmt.Errorf("unsupported extension kind %q", manifest.Kind)
	}
	switch manifest.Source {
	case SourceOfficial, SourceLocal, SourceCommunity:
	default:
		return fmt.Errorf("unsupported extension source %q", manifest.Source)
	}
	if err := validateLocalized("name", manifest.Name); err != nil {
		return err
	}
	if err := validateLocalized("summary", manifest.Summary); err != nil {
		return err
	}
	if manifest.Compatibility.APIMin < 0 || manifest.Compatibility.APIMax < manifest.Compatibility.APIMin {
		return fmt.Errorf("invalid API compatibility range")
	}
	if len(manifest.Compatibility.Platforms) == 0 || len(manifest.Compatibility.Architectures) == 0 {
		return fmt.Errorf("at least one platform and architecture are required")
	}
	if err := validateTokenSet("platform", manifest.Compatibility.Platforms); err != nil {
		return err
	}
	if err := validateTokenSet("architecture", manifest.Compatibility.Architectures); err != nil {
		return err
	}

	seenPermissions := map[string]struct{}{}
	for _, permission := range manifest.Permissions {
		if !keyPattern.MatchString(permission) {
			return fmt.Errorf("invalid permission %q", permission)
		}
		if _, known := knownPermissions[permission]; !known {
			return fmt.Errorf("unsupported permission %q", permission)
		}
		if _, duplicate := seenPermissions[permission]; duplicate {
			return fmt.Errorf("duplicate permission %q", permission)
		}
		seenPermissions[permission] = struct{}{}
	}
	if manifest.Kind == KindAddon && len(manifest.Permissions) != 0 {
		return fmt.Errorf("ADDON manifests cannot request runtime plugin permissions")
	}

	seenCapabilities := map[string]struct{}{}
	for _, capability := range manifest.Capabilities {
		if !keyPattern.MatchString(capability) {
			return fmt.Errorf("invalid capability %q", capability)
		}
		if _, duplicate := seenCapabilities[capability]; duplicate {
			return fmt.Errorf("duplicate capability %q", capability)
		}
		seenCapabilities[capability] = struct{}{}
	}

	seenDependencies := map[string]struct{}{}
	for _, dependency := range manifest.Dependencies {
		if !extensionIDPattern.MatchString(dependency.ExtensionID) {
			return fmt.Errorf("invalid dependency extension_id %q", dependency.ExtensionID)
		}
		if dependency.ExtensionID == manifest.ExtensionID {
			return fmt.Errorf("extension cannot depend on itself")
		}
		if _, duplicate := seenDependencies[dependency.ExtensionID]; duplicate {
			return fmt.Errorf("duplicate dependency %q", dependency.ExtensionID)
		}
		seenDependencies[dependency.ExtensionID] = struct{}{}
		if dependency.MinVersion != "" && !versionPattern.MatchString(dependency.MinVersion) {
			return fmt.Errorf("dependency %s has invalid min_version", dependency.ExtensionID)
		}
		if dependency.MaxVersion != "" && !versionPattern.MatchString(dependency.MaxVersion) {
			return fmt.Errorf("dependency %s has invalid max_version", dependency.ExtensionID)
		}
		if dependency.MinVersion != "" && dependency.MaxVersion != "" && compareSemanticVersions(dependency.MinVersion, dependency.MaxVersion) > 0 {
			return fmt.Errorf("dependency %s has min_version greater than max_version", dependency.ExtensionID)
		}
	}

	if manifest.Kind == KindAddon && manifest.Runtime != nil {
		return fmt.Errorf("ADDON manifests cannot declare a plugin runtime contract")
	}
	if manifest.Runtime != nil {
		if err := validateRuntimeContract(manifest); err != nil {
			return err
		}
	}
	return nil
}

func validateRuntimeContract(manifest Manifest) error {
	runtime := manifest.Runtime
	if runtime == nil {
		return nil
	}
	if manifest.Kind != KindPlugin {
		return fmt.Errorf("runtime contract is only valid for PLUGIN manifests")
	}
	if runtime.Protocol != RuntimeProtocolPluginV1 {
		return fmt.Errorf("unsupported runtime protocol %q", runtime.Protocol)
	}
	if runtime.Artifact != RuntimeArtifactNativeExecutable {
		return fmt.Errorf("unsupported runtime artifact %q", runtime.Artifact)
	}
	if len(manifest.Capabilities) == 0 {
		return fmt.Errorf("PLUGIN runtime contract requires at least one declared capability")
	}
	if len(runtime.CapabilityPermissions) != len(manifest.Capabilities) {
		return fmt.Errorf("runtime capability_permissions must map every declared capability exactly once")
	}
	declaredPermissions := make(map[string]struct{}, len(manifest.Permissions))
	for _, permission := range manifest.Permissions {
		declaredPermissions[permission] = struct{}{}
	}
	usedPermissions := make(map[string]struct{}, len(manifest.Permissions))
	for _, capability := range manifest.Capabilities {
		permissions, ok := runtime.CapabilityPermissions[capability]
		if !ok {
			return fmt.Errorf("runtime capability_permissions is missing capability %q", capability)
		}
		seen := map[string]struct{}{}
		for _, permission := range permissions {
			if _, declared := declaredPermissions[permission]; !declared {
				return fmt.Errorf("runtime capability %q references undeclared permission %q", capability, permission)
			}
			if _, duplicate := seen[permission]; duplicate {
				return fmt.Errorf("runtime capability %q repeats permission %q", capability, permission)
			}
			seen[permission] = struct{}{}
			usedPermissions[permission] = struct{}{}
		}
	}
	for capability := range runtime.CapabilityPermissions {
		if _, declared := seenCapability(manifest.Capabilities, capability); !declared {
			return fmt.Errorf("runtime capability_permissions contains undeclared capability %q", capability)
		}
	}
	for _, permission := range manifest.Permissions {
		if _, used := usedPermissions[permission]; !used {
			return fmt.Errorf("runtime contract requests unused permission %q", permission)
		}
	}
	return nil
}

func seenCapability(values []string, value string) (string, bool) {
	for _, item := range values {
		if item == value {
			return item, true
		}
	}
	return "", false
}

func normalizeManifest(manifest Manifest) Manifest {
	manifest.ExtensionID = strings.ToLower(strings.TrimSpace(manifest.ExtensionID))
	manifest.Version = strings.TrimSpace(manifest.Version)
	manifest.Kind = Kind(strings.ToUpper(strings.TrimSpace(string(manifest.Kind))))
	manifest.Source = Source(strings.ToUpper(strings.TrimSpace(string(manifest.Source))))
	manifest.Name.EN = strings.TrimSpace(manifest.Name.EN)
	manifest.Name.ArIQ = strings.TrimSpace(manifest.Name.ArIQ)
	manifest.Summary.EN = strings.TrimSpace(manifest.Summary.EN)
	manifest.Summary.ArIQ = strings.TrimSpace(manifest.Summary.ArIQ)
	for i := range manifest.Compatibility.Platforms {
		manifest.Compatibility.Platforms[i] = strings.ToLower(strings.TrimSpace(manifest.Compatibility.Platforms[i]))
	}
	for i := range manifest.Compatibility.Architectures {
		manifest.Compatibility.Architectures[i] = strings.ToLower(strings.TrimSpace(manifest.Compatibility.Architectures[i]))
	}
	for i := range manifest.Permissions {
		manifest.Permissions[i] = strings.ToLower(strings.TrimSpace(manifest.Permissions[i]))
	}
	for i := range manifest.Capabilities {
		manifest.Capabilities[i] = strings.ToLower(strings.TrimSpace(manifest.Capabilities[i]))
	}
	for i := range manifest.Dependencies {
		manifest.Dependencies[i].ExtensionID = strings.ToLower(strings.TrimSpace(manifest.Dependencies[i].ExtensionID))
		manifest.Dependencies[i].MinVersion = strings.TrimSpace(manifest.Dependencies[i].MinVersion)
		manifest.Dependencies[i].MaxVersion = strings.TrimSpace(manifest.Dependencies[i].MaxVersion)
	}
	if manifest.Runtime != nil {
		manifest.Runtime.Protocol = strings.ToLower(strings.TrimSpace(manifest.Runtime.Protocol))
		manifest.Runtime.Artifact = strings.ToLower(strings.TrimSpace(manifest.Runtime.Artifact))
		for capability, permissions := range manifest.Runtime.CapabilityPermissions {
			for index := range permissions {
				permissions[index] = strings.ToLower(strings.TrimSpace(permissions[index]))
			}
			sort.Strings(permissions)
			manifest.Runtime.CapabilityPermissions[capability] = permissions
		}
	}
	sort.Strings(manifest.Compatibility.Platforms)
	sort.Strings(manifest.Compatibility.Architectures)
	sort.Strings(manifest.Permissions)
	sort.Strings(manifest.Capabilities)
	sort.Slice(manifest.Dependencies, func(i, j int) bool { return manifest.Dependencies[i].ExtensionID < manifest.Dependencies[j].ExtensionID })
	return manifest
}

func validateLocalized(label string, value LocalizedText) error {
	if strings.TrimSpace(value.EN) == "" || strings.TrimSpace(value.ArIQ) == "" {
		return fmt.Errorf("extension %s requires English and ar-IQ text", label)
	}
	for _, r := range value.ArIQ {
		if unicode.In(r, unicode.Arabic) {
			return nil
		}
	}
	return fmt.Errorf("extension %s ar-IQ text must contain Arabic characters", label)
}

func validateTokenSet(label string, values []string) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		if !tokenPattern.MatchString(value) {
			return fmt.Errorf("invalid %s %q", label, value)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("duplicate %s %q", label, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
