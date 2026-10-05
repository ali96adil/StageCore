package deviceprofile

import (
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/stagelaser"
)

func TestBuiltinCatalogIncludesStageLaser(t *testing.T) {
	catalog := BuiltinCatalog()
	profile, err := catalog.Get(stagelaser.ProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Source != SourceOfficial || profile.Kind != KindDevice {
		t.Fatalf("StageLaser profile source/kind=%s/%s", profile.Source, profile.Kind)
	}
	if profile.Target == nil || profile.Target.LogicalType != stagelaser.LogicalTargetType {
		t.Fatalf("StageLaser target=%+v", profile.Target)
	}
	for _, field := range profile.ConnectionFields {
		if field.Key == "host" || field.Key == "url" || field.Key == "ip" {
			t.Fatalf("StageLaser profile unexpectedly requires network address field %q", field.Key)
		}
	}
}

func TestStageLaserDiscoveryProfileMatch(t *testing.T) {
	catalog := BuiltinCatalog()
	profile, err := catalog.Choose(Observation{Attributes: map[string]string{
		"profile_id":   stagelaser.ProfileID,
		"api_protocol": stagelaser.TransportProtocolVersion,
		"service_type": stagelaser.DiscoveryServiceType,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if profile.ID != stagelaser.ProfileID {
		t.Fatalf("matched profile=%q want %q", profile.ID, stagelaser.ProfileID)
	}
}

func TestStageLaserTargetMaterializesOnlyStableIdentity(t *testing.T) {
	catalog := BuiltinCatalog()
	target, err := catalog.Materialize(stagelaser.ProfileID, map[string]any{
		"device_id": "stagelaser-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if target.LogicalType != stagelaser.LogicalTargetType {
		t.Fatalf("logical type=%q", target.LogicalType)
	}
	var config map[string]any
	if err := json.Unmarshal(target.Configuration, &config); err != nil {
		t.Fatal(err)
	}
	if config["device_id"] != "stagelaser-01" {
		t.Fatalf("materialized config=%v", config)
	}
	if len(config) != 1 {
		t.Fatalf("StageLaser target must not persist endpoint details: %v", config)
	}
}

func TestStageLaserResyncIsNotGenericAction(t *testing.T) {
	profile, err := BuiltinCatalog().Get(stagelaser.ProfileID)
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range profile.Capabilities {
		if capability.Key == stagelaser.CapabilityStateResync && len(capability.Actions) != 0 {
			t.Fatalf("state resync must be diagnostics-only, actions=%v", capability.Actions)
		}
	}
}
