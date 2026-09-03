package operatorweb

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

type featureLocalizationManifest struct {
	ContractVersion  int                          `json:"contract_version"`
	RequiredLocales []string                     `json:"required_locales"`
	Features         []featureLocalizationFeature `json:"features"`
}

type featureLocalizationFeature struct {
	FeatureID        string                      `json:"feature_id"`
	LocalizationMode string                      `json:"localization_mode"`
	OwnerAssets      []string                    `json:"owner_assets"`
	Name             map[string]string           `json:"name"`
	Summary          map[string]string           `json:"summary"`
	Actions          []featureLocalizationAction `json:"actions"`
	UIKeys           []string                    `json:"ui_keys"`
}

type featureLocalizationAction struct {
	ActionID string            `json:"action_id"`
	Label    map[string]string `json:"label"`
}

var featureIDPattern = regexp.MustCompile(`^(?:F-[0-9]{3}|CORE-[A-Z0-9-]+)$`)
var operatorAssetPattern = regexp.MustCompile(`(?:src|href)="/([^"?#]+\.(?:js|css))"`)

func TestFeatureLocalizationContract(t *testing.T) {
	manifest := featureLocalizationManifest{ContractVersion: 1, RequiredLocales: []string{"en", "ar-IQ"}}
	for _, path := range []string{"feature_localization_manifest.json", "feature_localization_manifest_f011.json"} {
		partBytes := mustReadOperatorContractFile(t, path)
		var part featureLocalizationManifest
		if err := json.Unmarshal(partBytes, &part); err != nil {
			t.Fatalf("decode feature localization manifest %s: %v", path, err)
		}
		if part.ContractVersion != manifest.ContractVersion {
			t.Fatalf("feature localization contract %s version=%d, want %d", path, part.ContractVersion, manifest.ContractVersion)
		}
		if !sameStringSet(part.RequiredLocales, manifest.RequiredLocales) {
			t.Fatalf("required locales in %s=%v, want %v", path, part.RequiredLocales, manifest.RequiredLocales)
		}
		manifest.Features = append(manifest.Features, part.Features...)
	}

	index := string(mustReadOperatorContractFile(t, "static/index.html"))
	localizationSource := string(mustReadOperatorContractFile(t, "static/localization.js"))
	legacyAllowed := map[string]bool{
		"CORE-OPERATOR":      true,
		"CORE-AUTH":          true,
		"CORE-CONFIGURATION": true,
		"CORE-PREFLIGHT":     true,
		"CORE-MEMORY":        true,
		"CORE-SECURITY":      true,
		"F-001":              true,
		"F-012":              true,
	}

	featureIDs := map[string]bool{}
	assetOwners := map[string]string{}
	for _, feature := range manifest.Features {
		if !featureIDPattern.MatchString(feature.FeatureID) {
			t.Errorf("invalid feature_id %q", feature.FeatureID)
		}
		if featureIDs[feature.FeatureID] {
			t.Errorf("duplicate feature_id %q", feature.FeatureID)
		}
		featureIDs[feature.FeatureID] = true

		if feature.LocalizationMode != "keyed" && feature.LocalizationMode != "legacy-source-text" {
			t.Errorf("%s localization_mode=%q", feature.FeatureID, feature.LocalizationMode)
		}
		if feature.LocalizationMode == "legacy-source-text" && !legacyAllowed[feature.FeatureID] {
			t.Errorf("%s cannot introduce legacy-source-text localization; new user-facing features must use keyed localization", feature.FeatureID)
		}

		validateLocalizedField(t, feature.FeatureID+" name", manifest.RequiredLocales, feature.Name)
		validateLocalizedField(t, feature.FeatureID+" summary", manifest.RequiredLocales, feature.Summary)
		if len(feature.Actions) == 0 {
			t.Errorf("%s must describe at least one user-facing action", feature.FeatureID)
		}
		actionIDs := map[string]bool{}
		for _, action := range feature.Actions {
			if strings.TrimSpace(action.ActionID) == "" {
				t.Errorf("%s has action with empty action_id", feature.FeatureID)
			}
			if actionIDs[action.ActionID] {
				t.Errorf("%s has duplicate action_id %q", feature.FeatureID, action.ActionID)
			}
			actionIDs[action.ActionID] = true
			validateLocalizedField(t, feature.FeatureID+" action "+action.ActionID, manifest.RequiredLocales, action.Label)
		}

		if len(feature.OwnerAssets) == 0 {
			t.Errorf("%s must own at least one Operator asset", feature.FeatureID)
		}
		ownerSources := make([]string, 0, len(feature.OwnerAssets))
		for _, asset := range feature.OwnerAssets {
			asset = strings.TrimPrefix(asset, "/")
			if previous, exists := assetOwners[asset]; exists {
				t.Errorf("operator asset %q is owned by both %s and %s", asset, previous, feature.FeatureID)
			}
			assetOwners[asset] = feature.FeatureID
			ownerSources = append(ownerSources, string(mustReadOperatorContractFile(t, asset)))
		}

		if feature.LocalizationMode == "keyed" {
			if len(feature.UIKeys) == 0 {
				t.Errorf("%s uses keyed localization but declares no ui_keys", feature.FeatureID)
			}
			for _, key := range feature.UIKeys {
				if strings.TrimSpace(key) == "" {
					t.Errorf("%s declares an empty localization key", feature.FeatureID)
					continue
				}
				if !sourceSetContains(ownerSources, key) {
					t.Errorf("%s localization key %q is not referenced by an owner asset", feature.FeatureID, key)
				}
				arabic, ok := localizedArabicValue(localizationSource, key)
				if !ok {
					arabic, ok = localizedArabicValueFromSources(ownerSources, key)
				}
				if !ok {
					t.Errorf("%s localization key %q has no Arabic dictionary entry in the shared or feature-owned catalog", feature.FeatureID, key)
				} else if !containsArabic(arabic) {
					t.Errorf("%s localization key %q Arabic value %q does not contain Arabic text", feature.FeatureID, key, arabic)
				}
			}
		}
	}

	for _, match := range operatorAssetPattern.FindAllStringSubmatch(index, -1) {
		asset := "static/" + match[1]
		if _, ok := assetOwners[asset]; !ok {
			t.Errorf("Operator asset %q has no feature localization owner; register it in a feature localization manifest", asset)
		}
	}
}

func validateLocalizedField(t *testing.T, label string, locales []string, values map[string]string) {
	t.Helper()
	for _, locale := range locales {
		value := strings.TrimSpace(values[locale])
		if value == "" {
			t.Errorf("%s missing %s text", label, locale)
			continue
		}
		if locale == "ar-IQ" && !containsArabic(value) {
			t.Errorf("%s ar-IQ value %q does not contain Arabic text", label, value)
		}
	}
}

func localizedArabicValue(source, key string) (string, bool) {
	pattern := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `"\s*:\s*(?:\{[^\n]*"ar-IQ"\s*:\s*)?"([^"]+)"`)
	match := pattern.FindStringSubmatch(source)
	if len(match) != 2 { return "", false }
	return match[1], true
}

func localizedArabicValueFromSources(sources []string, key string) (string, bool) {
	for _, source := range sources {
		if value, ok := localizedArabicValue(source, key); ok { return value, true }
	}
	return "", false
}

func sourceSetContains(sources []string, needle string) bool {
	for _, source := range sources { if strings.Contains(source, needle) { return true } }
	return false
}

func containsArabic(value string) bool {
	for _, r := range value { if unicode.In(r, unicode.Arabic) { return true } }
	return false
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) { return false }
	seen := make(map[string]int, len(a))
	for _, value := range a { seen[value]++ }
	for _, value := range b { seen[value]-- }
	for _, count := range seen { if count != 0 { return false } }
	return true
}

func mustReadOperatorContractFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil { t.Fatalf("read %s: %v", path, err) }
	return content
}
