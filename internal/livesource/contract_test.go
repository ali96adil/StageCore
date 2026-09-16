package livesource

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDecodeTargetConfigNormalizesDefaults(t *testing.T) {
	cfg, err := DecodeTargetConfig(json.RawMessage(`{
		"contract_version":1,
		"source_id":"cam-main",
		"name":"Main camera",
		"source_class":"USB_CAPTURE",
		"machine_role_id":"role-video",
		"adapter_config":{"device_uid":"capture-1"},
		"required":true,
		"desired_enabled":true
	}`))
	if err != nil {
		t.Fatalf("DecodeTargetConfig: %v", err)
	}
	if cfg.SourceClass != SourceUSBCapture || cfg.ObservationMaxAge() != DefaultObservationMaxAge {
		t.Fatalf("unexpected normalized config: %+v", cfg)
	}
	if !HasCapability(cfg.RequiredCapabilities, CapabilityInspect) || !HasCapability(cfg.RequiredCapabilities, CapabilityOpen) {
		t.Fatalf("desired enabled source must default to inspect+open capabilities: %v", cfg.RequiredCapabilities)
	}
}

func TestDecodeTargetConfigRejectsUnknownAndUnsafeShapes(t *testing.T) {
	cases := []string{
		`{"contract_version":2,"source_id":"x","name":"x","source_class":"USB_CAPTURE","machine_role_id":"r"}`,
		`{"contract_version":1,"source_id":"x","name":"x","source_class":"MAGIC","machine_role_id":"r"}`,
		`{"contract_version":1,"source_id":"x","name":"x","source_class":"USB_CAPTURE","machine_role_id":"r","adapter_config":[]}`,
		`{"contract_version":1,"source_id":"x","name":"x","source_class":"USB_CAPTURE","machine_role_id":"r","unknown":true}`,
		`{"contract_version":1,"source_id":"x","name":"x","source_class":"USB_CAPTURE","machine_role_id":"r","required_capabilities":["osc.send"]}`,
		`{"contract_version":1,"source_id":"x","name":"x","source_class":"USB_CAPTURE","machine_role_id":"r","observation_max_age_ms":30001}`,
	}
	for _, raw := range cases {
		if _, err := DecodeTargetConfig(json.RawMessage(raw)); err == nil {
			t.Fatalf("expected rejection for %s", raw)
		}
	}
}

func TestEncodeCommandBindsImmutableDescriptor(t *testing.T) {
	cfg, err := DecodeTargetConfig(json.RawMessage(`{
		"contract_version":1,
		"source_id":"camera-a",
		"name":"Camera A",
		"source_class":"LOCAL_CAMERA",
		"machine_role_id":"role-a",
		"adapter_config":{"unique_id":"abc"},
		"required":false,
		"desired_enabled":false
	}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeCommand(cfg, json.RawMessage(`{"layer_id":"live-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	var command Command
	if err := json.Unmarshal(raw, &command); err != nil {
		t.Fatal(err)
	}
	if command.SourceID != "camera-a" || command.SourceClass != SourceLocalCamera {
		t.Fatalf("immutable source descriptor missing: %+v", command)
	}
	if string(command.Parameters) != `{"layer_id":"live-1"}` {
		t.Fatalf("command parameters changed: %s", command.Parameters)
	}
}

func TestDecodeObservationRequiresVersionedReadiness(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	raw, _ := json.Marshal(Observation{
		ContractVersion: ContractVersion1,
		SourceID: "camera-a",
		Readiness: ReadinessReady,
		ObservedAt: now,
		Capabilities: []string{CapabilityInspect, CapabilityOpen},
		Generation: 4,
	})
	observation, err := DecodeObservation(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if observation.SourceID != "camera-a" || observation.Readiness != ReadinessReady || observation.Generation != 4 {
		t.Fatalf("unexpected observation: %+v", observation)
	}
	if _, err := DecodeObservation(`{"contract_version":1,"source_id":"camera-a","readiness":"MAGIC","observed_at":"2026-09-16T00:00:00Z"}`); err == nil {
		t.Fatal("expected invalid readiness rejection")
	}
}
