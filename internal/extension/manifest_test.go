package extension

import (
	"strings"
	"testing"
)

func TestParseManifestRequiresStrictBilingualTrustedShape(t *testing.T) {
	raw := []byte(`{
		"schema_version":1,
		"extension_id":"example.osc-plugin",
		"version":"1.2.3",
		"kind":"PLUGIN",
		"source":"LOCAL",
		"name":{"en":"Example OSC Plugin","ar-IQ":"إضافة OSC تجريبية"},
		"summary":{"en":"Sends OSC messages.","ar-IQ":"ترسل رسائل OSC إلى أجهزة المسرح."},
		"compatibility":{"api_min":1,"api_max":1,"platforms":["linux"],"architectures":["arm64"]},
		"permissions":["network.udp.send"],
		"capabilities":["osc.send"]
	}`)
	manifest, canonical, err := ParseManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ExtensionID != "example.osc-plugin" || manifest.Name.ArIQ == "" || len(canonical) == 0 {
		t.Fatalf("manifest=%+v canonical=%q", manifest, canonical)
	}

	withUnknown := strings.Replace(string(raw), `"capabilities":["osc.send"]`, `"capabilities":["osc.send"],"secret":"do-not-store"`, 1)
	if withUnknown == string(raw) {
		t.Fatal("test setup did not inject unknown field")
	}
	if _, _, err := ParseManifest([]byte(withUnknown)); err == nil {
		t.Fatal("unknown manifest field was accepted")
	}

	withoutArabic := strings.Replace(string(raw), "إضافة OSC تجريبية", "OSC plugin", 1)
	withoutArabic = strings.Replace(withoutArabic, "ترسل رسائل OSC إلى أجهزة المسرح.", "Sends OSC.", 1)
	if _, _, err := ParseManifest([]byte(withoutArabic)); err == nil {
		t.Fatal("manifest without ar-IQ content was accepted")
	}
}

func TestManifestRejectsUnknownPermissionAndDependencyConflicts(t *testing.T) {
	base := Manifest{
		SchemaVersion: ManifestSchemaVersion,
		ExtensionID: "example.plugin",
		Version: "1.0.0",
		Kind: KindPlugin,
		Source: SourceLocal,
		Name: LocalizedText{EN: "Example", ArIQ: "مثال"},
		Summary: LocalizedText{EN: "Example extension", ArIQ: "إضافة تجريبية"},
		Compatibility: Compatibility{APIMin: 1, APIMax: 1, Platforms: []string{"linux"}, Architectures: []string{"arm64"}},
	}
	base.Permissions = []string{"filesystem.root"}
	if err := ValidateManifest(base); err == nil {
		t.Fatal("unknown permission was accepted")
	}
	base.Permissions = nil
	base.Dependencies = []Dependency{{ExtensionID: "example.plugin", MinVersion: "1.0.0"}}
	if err := ValidateManifest(base); err == nil {
		t.Fatal("self dependency was accepted")
	}
}
