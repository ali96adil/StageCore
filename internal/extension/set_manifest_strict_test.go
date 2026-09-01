package extension

import "testing"

func TestParseExtensionSetManifestRejectsValidJSONUnknownField(t *testing.T) {
	raw := []byte(`{"format":"stagecore-extension-set-v1","schema_version":1,"extensions":[],"unexpected":true}`)
	if string(raw[:2]) == `{"` {
		t.Fatal("test fixture must be valid unescaped JSON")
	}
	if _, err := ParseExtensionSetManifest(raw); err == nil {
		t.Fatal("parser accepted a valid manifest with an unknown field")
	}
}
