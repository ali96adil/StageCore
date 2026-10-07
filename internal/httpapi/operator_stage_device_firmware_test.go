package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/deviceupdate"
	"github.com/ali96adil/StageCore/internal/stagelaser"
)

func readyStageLaserDevice(t *testing.T) deviceexperience.Device {
	t.Helper()
	observation := stagelaser.Observation{
		SchemaVersion: stagelaser.SchemaVersion1,
		FirmwareVersion: "0.1.0",
		ControlContractVersion: stagelaser.ControlContractVersion,
		ArmState: stagelaser.ArmDisarmed,
		LogicalState: stagelaser.StateOff,
		StateQuality: stagelaser.StateQualityTracked,
		DriverKind: stagelaser.DriverMechanicalRelay,
		Limits: stagelaser.DefaultMechanicalLimits(),
	}
	raw, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	return deviceexperience.Device{
		ID: "23c45a07-7286-4afc-91d9-7e54df72aeee",
		ProfileID: stagelaser.ProfileID,
		Kind: deviceexperience.DeviceGeneric,
		ClientVersion: "0.1.0",
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Enabled: true,
		Runtime: &deviceexperience.RuntimeState{
			Connection: deviceexperience.ConnectionOnline,
			Readiness: deviceexperience.ReadinessReady,
			ObservedState: raw,
		},
	}
}

func qualifiedStageLaserArtifact(device deviceexperience.Device) deviceupdate.ArtifactMetadata {
	return deviceupdate.ArtifactMetadata{
		ArtifactID: "018f2744-0cb0-7bf6-9637-4a3a467a7a31",
		DeviceID: device.ID,
		ProfileID: device.ProfileID,
		Version: "0.2.0",
		SourceRevision: strings.Repeat("a", 40),
		Qualification: deviceupdate.QualificationQualified,
		SizeBytes: 123456,
		SHA256: strings.Repeat("b", 64),
		CreatedAt: time.Date(2026, 10, 7, 11, 0, 0, 0, time.UTC),
	}
}

func TestStageLaserFirmwareMaintenanceRequiresKnownStableOff(t *testing.T) {
	device := readyStageLaserDevice(t)
	if err := firmwareMaintenanceStateReady(device); err != nil {
		t.Fatalf("ready StageLaser rejected: %v", err)
	}

	var observation stagelaser.Observation
	if err := json.Unmarshal(device.Runtime.ObservedState, &observation); err != nil {
		t.Fatal(err)
	}
	observation.LogicalState = stagelaser.StateUnknown
	observation.StateQuality = stagelaser.StateQualityUnknown
	observation.ResyncRequired = true
	raw, _ := json.Marshal(observation)
	device.Runtime.ObservedState = raw
	if err := firmwareMaintenanceStateReady(device); err == nil {
		t.Fatal("UNKNOWN StageLaser unexpectedly accepted")
	}

	device = readyStageLaserDevice(t)
	_ = json.Unmarshal(device.Runtime.ObservedState, &observation)
	observation.ArmState = stagelaser.ArmArmed
	raw, _ = json.Marshal(observation)
	device.Runtime.ObservedState = raw
	if err := firmwareMaintenanceStateReady(device); err == nil {
		t.Fatal("ARMED StageLaser unexpectedly accepted")
	}
}

func TestStageLaserFirmwareManifestRequiresRollbackAndExactArtifact(t *testing.T) {
	device := readyStageLaserDevice(t)
	artifact := qualifiedStageLaserArtifact(device)
	now := time.Date(2026, 10, 7, 11, 30, 0, 0, time.UTC)

	manifest, err := buildStageDeviceFirmwareManifest(device, artifact, now)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.RollbackRequired {
		t.Fatal("StageLaser manifest did not require rollback")
	}
	if manifest.ArtifactPath != deviceupdate.ArtifactPath(artifact.ArtifactID) {
		t.Fatalf("artifact path = %q", manifest.ArtifactPath)
	}
	if manifest.ExpiresAt.Sub(manifest.IssuedAt) != operatorFirmwareManifestTTL {
		t.Fatalf("manifest TTL = %s", manifest.ExpiresAt.Sub(manifest.IssuedAt))
	}

	artifact.ProfileID = "stagecore.other"
	if _, err := buildStageDeviceFirmwareManifest(device, artifact, now); err == nil {
		t.Fatal("cross-profile artifact unexpectedly accepted")
	}
}

func TestFirmwareMaintenanceRequiresOnlineReady(t *testing.T) {
	device := readyStageLaserDevice(t)
	device.Runtime.Connection = deviceexperience.ConnectionOffline
	if err := firmwareMaintenanceStateReady(device); err == nil {
		t.Fatal("offline device unexpectedly accepted")
	}
}
