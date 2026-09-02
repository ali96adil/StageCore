package executionenv

import (
	"fmt"
	"sort"
	"strings"
)

type ReadinessStatus string

const (
	ReadinessPass  ReadinessStatus = "PASS"
	ReadinessWarn  ReadinessStatus = "WARN"
	ReadinessBlock ReadinessStatus = "BLOCK"
)

type ApplicationObservation struct {
	Present                  bool
	ObservedVersion          string
	VersionConstraintSatisfied *bool
}

type AssetObservation struct {
	Key         string
	Present     bool
	Inspectable bool
	ContentHash string
	SizeBytes   *int64
}

type ExternalExtensionObservation struct {
	Key                        string
	Present                    bool
	ObservedVersion            string
	VersionConstraintSatisfied *bool
}

type BindingObservation struct {
	Key     string
	Present bool
}

type Observation struct {
	OS           string
	Architecture string
	Application  ApplicationObservation
	Assets       []AssetObservation
	Extensions   []ExternalExtensionObservation
	Bindings     []BindingObservation
}

type ReadinessCheck struct {
	Key      string          `json:"key"`
	Category string          `json:"category"`
	Status   ReadinessStatus `json:"status"`
	Summary  string          `json:"summary"`
	Detail   string          `json:"detail,omitempty"`
}

type ReadinessReport struct {
	Status ReadinessStatus `json:"status"`
	Checks []ReadinessCheck `json:"checks"`
}

func EvaluateReadiness(manifest Manifest, observation Observation) (ReadinessReport, error) {
	normalized, err := Normalize(manifest)
	if err != nil {
		return ReadinessReport{}, err
	}
	assets, err := indexAssetObservations(observation.Assets)
	if err != nil {
		return ReadinessReport{}, err
	}
	extensions, err := indexExtensionObservations(observation.Extensions)
	if err != nil {
		return ReadinessReport{}, err
	}
	bindings, err := indexBindingObservations(observation.Bindings)
	if err != nil {
		return ReadinessReport{}, err
	}

	report := ReadinessReport{Status: ReadinessPass, Checks: []ReadinessCheck{}}
	osName := strings.ToLower(strings.TrimSpace(observation.OS))
	architecture := strings.ToLower(strings.TrimSpace(observation.Architecture))
	hostSupported := false
	for _, host := range normalized.Application.Hosts {
		if host.OS == osName && host.Architecture == architecture {
			hostSupported = true
			break
		}
	}
	if hostSupported {
		report.add(ReadinessPass, "application.host", "application", "Host platform is supported", osName+"/"+architecture)
	} else {
		report.add(ReadinessBlock, "application.host", "application", "Host platform is not supported", osName+"/"+architecture)
	}

	if !observation.Application.Present {
		report.add(ReadinessBlock, "application.present", "application", "Required application is missing", normalized.Application.Name)
	} else {
		report.add(ReadinessPass, "application.present", "application", "Required application is present", normalized.Application.Name)
		switch compatibilityState(observation.Application.VersionConstraintSatisfied) {
		case compatibilityPass:
			report.add(ReadinessPass, "application.version", "application", "Application version satisfies requirement", versionDetail(observation.Application.ObservedVersion, normalized.Application.VersionConstraint))
		case compatibilityFail:
			report.add(ReadinessBlock, "application.version", "application", "Application version does not satisfy requirement", versionDetail(observation.Application.ObservedVersion, normalized.Application.VersionConstraint))
		default:
			report.add(ReadinessBlock, "application.version", "application", "Application version compatibility was not verified", versionDetail(observation.Application.ObservedVersion, normalized.Application.VersionConstraint))
		}
	}

	for _, requirement := range normalized.Assets {
		observation, ok := assets[requirement.Key]
		key := "asset." + requirement.Key
		if !ok || !observation.Present {
			report.add(ReadinessBlock, key, "asset", "Required execution asset is missing", requirement.Name)
			continue
		}
		if requirement.CapturePolicy == CaptureReferenceOnly {
			detail := requirement.Locator
			if observation.Inspectable {
				detail += " (present and inspectable; content is reference-only)"
			} else {
				detail += " (present but content cannot be verified)"
			}
			report.add(ReadinessWarn, key, "asset", "Reference-only asset limits reproducibility", detail)
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(observation.ContentHash), requirement.ContentHash) {
			report.add(ReadinessBlock, key, "asset", "Execution asset content hash mismatch", requirement.Name)
			continue
		}
		if observation.SizeBytes == nil || requirement.SizeBytes == nil || *observation.SizeBytes != *requirement.SizeBytes {
			report.add(ReadinessBlock, key, "asset", "Execution asset size mismatch", requirement.Name)
			continue
		}
		report.add(ReadinessPass, key, "asset", "Execution asset integrity verified", requirement.Name)
	}

	for _, requirement := range normalized.Extensions {
		observation, ok := extensions[requirement.Key]
		key := "extension." + requirement.Key
		if !ok || !observation.Present {
			if requirement.Required {
				report.add(ReadinessBlock, key, "extension", "Required external extension is missing", requirement.Name)
			} else {
				report.add(ReadinessWarn, key, "extension", "Optional external extension is missing", requirement.Name)
			}
			continue
		}
		switch compatibilityState(observation.VersionConstraintSatisfied) {
		case compatibilityPass:
			report.add(ReadinessPass, key, "extension", "External extension version satisfies requirement", versionDetail(observation.ObservedVersion, requirement.VersionConstraint))
		case compatibilityFail:
			if requirement.Required {
				report.add(ReadinessBlock, key, "extension", "Required external extension version is incompatible", versionDetail(observation.ObservedVersion, requirement.VersionConstraint))
			} else {
				report.add(ReadinessWarn, key, "extension", "Optional external extension version is incompatible", versionDetail(observation.ObservedVersion, requirement.VersionConstraint))
			}
		default:
			if requirement.Required {
				report.add(ReadinessBlock, key, "extension", "Required external extension version was not verified", versionDetail(observation.ObservedVersion, requirement.VersionConstraint))
			} else {
				report.add(ReadinessWarn, key, "extension", "Optional external extension version was not verified", versionDetail(observation.ObservedVersion, requirement.VersionConstraint))
			}
		}
	}

	for _, requirement := range normalized.Bindings {
		observation, ok := bindings[requirement.Key]
		key := "binding." + requirement.Key
		if !ok || !observation.Present {
			if requirement.Required {
				report.add(ReadinessBlock, key, "binding", "Required execution binding is missing", requirement.Name)
			} else {
				report.add(ReadinessWarn, key, "binding", "Optional execution binding is missing", requirement.Name)
			}
			continue
		}
		report.add(ReadinessPass, key, "binding", "Execution binding is present", requirement.Name)
	}

	sort.SliceStable(report.Checks, func(i, j int) bool {
		return report.Checks[i].Key < report.Checks[j].Key
	})
	return report, nil
}

func (r *ReadinessReport) add(status ReadinessStatus, key, category, summary, detail string) {
	r.Checks = append(r.Checks, ReadinessCheck{Key: key, Category: category, Status: status, Summary: summary, Detail: detail})
	if readinessRank(status) > readinessRank(r.Status) {
		r.Status = status
	}
}

func readinessRank(status ReadinessStatus) int {
	switch status {
	case ReadinessBlock:
		return 2
	case ReadinessWarn:
		return 1
	default:
		return 0
	}
}

type compatibility int

const (
	compatibilityUnknown compatibility = iota
	compatibilityPass
	compatibilityFail
)

func compatibilityState(value *bool) compatibility {
	if value == nil {
		return compatibilityUnknown
	}
	if *value {
		return compatibilityPass
	}
	return compatibilityFail
}

func versionDetail(observed, constraint string) string {
	observed = strings.TrimSpace(observed)
	if observed == "" {
		observed = "unknown"
	}
	return fmt.Sprintf("observed=%s required=%s", observed, constraint)
}

func indexAssetObservations(values []AssetObservation) (map[string]AssetObservation, error) {
	result := make(map[string]AssetObservation, len(values))
	for _, value := range values {
		key := strings.TrimSpace(value.Key)
		if key == "" {
			return nil, fmt.Errorf("asset observation key is required")
		}
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("duplicate asset observation key %q", key)
		}
		value.Key = key
		result[key] = value
	}
	return result, nil
}

func indexExtensionObservations(values []ExternalExtensionObservation) (map[string]ExternalExtensionObservation, error) {
	result := make(map[string]ExternalExtensionObservation, len(values))
	for _, value := range values {
		key := strings.TrimSpace(value.Key)
		if key == "" {
			return nil, fmt.Errorf("extension observation key is required")
		}
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("duplicate extension observation key %q", key)
		}
		value.Key = key
		result[key] = value
	}
	return result, nil
}

func indexBindingObservations(values []BindingObservation) (map[string]BindingObservation, error) {
	result := make(map[string]BindingObservation, len(values))
	for _, value := range values {
		key := strings.TrimSpace(value.Key)
		if key == "" {
			return nil, fmt.Errorf("binding observation key is required")
		}
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("duplicate binding observation key %q", key)
		}
		value.Key = key
		result[key] = value
	}
	return result, nil
}
