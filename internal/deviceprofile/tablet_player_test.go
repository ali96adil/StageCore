package deviceprofile

import "testing"

func TestBuiltinCatalogIncludesTabletPlayer(t *testing.T) {
	catalog := BuiltinCatalog()
	profile, err := catalog.Get("stagecore.tablet-player")
	if err != nil {
		t.Fatalf("tablet player profile missing: %v", err)
	}
	if profile.Source != SourceOfficial || profile.Kind != KindDevice {
		t.Fatalf("unexpected tablet profile source/kind: %s/%s", profile.Source, profile.Kind)
	}
	if len(profile.Capabilities) < 10 {
		t.Fatalf("tablet profile capabilities = %d, want >= 10", len(profile.Capabilities))
	}
	matches := catalog.Match(Observation{Attributes: map[string]string{
		"device_kind":      "TABLET_PLAYER",
		"protocol_version": "stagecore.device/1",
	}})
	if len(matches) == 0 || matches[0].ProfileID != "stagecore.tablet-player" {
		t.Fatalf("tablet observation did not resolve to official tablet profile: %#v", matches)
	}
}
