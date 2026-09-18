package deviceprofile

import (
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/lightingnode"
)

func TestBuiltinCatalogIncludesESP32DMXLightingNode(t *testing.T) {
	catalog := BuiltinCatalog()
	profile, err := catalog.Get(lightingnode.ProfileID)
	if err != nil {
		t.Fatalf("lighting node profile missing: %v", err)
	}
	if profile.Source != SourceOfficial || profile.Kind != KindDevice {
		t.Fatalf("unexpected lighting profile source/kind: %s/%s", profile.Source, profile.Kind)
	}
	if len(profile.Capabilities) != len(lightingnode.CapabilityKeys()) {
		t.Fatalf("lighting capabilities=%d want=%d", len(profile.Capabilities), len(lightingnode.CapabilityKeys()))
	}

	matches := catalog.Match(Observation{Attributes: map[string]string{
		"profile_id":       lightingnode.ProfileID,
		"protocol_version": "stagecore.device/1",
	}})
	if len(matches) == 0 || matches[0].ProfileID != lightingnode.ProfileID {
		t.Fatalf("lighting observation did not resolve to official profile: %#v", matches)
	}

	target, err := catalog.Materialize(lightingnode.ProfileID, map[string]any{"device_id": "lighting-01"})
	if err != nil {
		t.Fatal(err)
	}
	if target.LogicalType != devicechannel.StageDeviceLogicalType {
		t.Fatalf("logical type=%q want=%q", target.LogicalType, devicechannel.StageDeviceLogicalType)
	}
	var config struct {
		DeviceID string `json:"device_id"`
	}
	if err := json.Unmarshal(target.Configuration, &config); err != nil {
		t.Fatal(err)
	}
	if config.DeviceID != "lighting-01" {
		t.Fatalf("device_id=%q", config.DeviceID)
	}
}
