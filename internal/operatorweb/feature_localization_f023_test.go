package operatorweb

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestF023AssistantLocalizationContract(t *testing.T) {
	manifestBytes := mustReadOperatorContractFile(t, "feature_localization_manifest_f023.json")
	var manifest featureLocalizationManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("decode F-023 localization manifest: %v", err)
	}
	if manifest.ContractVersion != 1 || !sameStringSet(manifest.RequiredLocales, []string{"en", "ar-IQ"}) {
		t.Fatalf("F-023 localization header=%+v", manifest)
	}
	if len(manifest.Features) != 1 || manifest.Features[0].FeatureID != "F-023" {
		t.Fatalf("F-023 features=%+v", manifest.Features)
	}
	feature := manifest.Features[0]
	if feature.LocalizationMode != "keyed" {
		t.Fatalf("F-023 localization_mode=%q", feature.LocalizationMode)
	}
	validateLocalizedField(t, "F-023 name", manifest.RequiredLocales, feature.Name)
	validateLocalizedField(t, "F-023 summary", manifest.RequiredLocales, feature.Summary)
	if len(feature.Actions) < 3 {
		t.Fatalf("F-023 actions=%v", feature.Actions)
	}
	ownerSources := make([]string, 0, len(feature.OwnerAssets))
	for _, asset := range feature.OwnerAssets {
		ownerSources = append(ownerSources, string(mustReadOperatorContractFile(t, strings.TrimPrefix(asset, "/"))))
	}
	if len(feature.UIKeys) == 0 {
		t.Fatal("F-023 must declare keyed UI strings")
	}
	for _, key := range feature.UIKeys {
		if !sourceSetContains(ownerSources, key) {
			t.Errorf("F-023 localization key %q is not referenced by an owner asset", key)
			continue
		}
		arabic, ok := localizedArabicValueFromSources(ownerSources, key)
		if !ok || !containsArabic(arabic) {
			t.Errorf("F-023 localization key %q has no Arabic value", key)
		}
	}
}
