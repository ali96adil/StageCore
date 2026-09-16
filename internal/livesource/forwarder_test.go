package livesource

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companion"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestForwarderBindsSourceToConfiguredMachineRole(t *testing.T) {
	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	s := store.New(handle.DB, clock.Real{})
	project, _, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Live test", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	role, err := s.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "video-renderer", DisplayName: "Video renderer",
		RequiredCapabilities: []string{CapabilityOpen, CapabilityInspect}, Required: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	var captured capability.Request
	backend := capability.ExecutorFunc(func(_ context.Context, req capability.Request) capability.Result {
		captured = req
		return capability.Result{}
	})
	forwarder := NewForwarder(s, backend)
	config, _ := json.Marshal(TargetConfig{
		ContractVersion: ContractVersion1, SourceID: "camera-main", Name: "Main camera",
		SourceClass: SourceUSBCapture, MachineRoleID: role.ID, AdapterConfig: json.RawMessage(`{"device_uid":"usb-1"}`),
		Required: true, DesiredEnabled: true,
	})
	parameters := json.RawMessage(`{"layer_id":"live-main"}`)
	forwarder.Execute(ctx, capability.Request{
		ExecutionID: "exec-1", ProjectID: project.ID, RuntimeSnapshotID: "snapshot-1",
		Capability: CapabilityOpen,
		Target: &capability.Target{AliasID: "alias-1", Ref: "camera", LogicalType: LogicalType, Configuration: config},
		Parameters: parameters,
	})

	if captured.Target == nil || captured.Target.LogicalType != companion.MachineRoleLogicalType || captured.Target.Ref != role.RoleKey {
		t.Fatalf("request was not rebound to configured Machine Role: %+v", captured.Target)
	}
	var roleConfig map[string]string
	if err := json.Unmarshal(captured.Target.Configuration, &roleConfig); err != nil {
		t.Fatal(err)
	}
	if roleConfig["machine_role_id"] != role.ID {
		t.Fatalf("wrong machine role binding: %v", roleConfig)
	}
	var command Command
	if err := json.Unmarshal(captured.Parameters, &command); err != nil {
		t.Fatal(err)
	}
	if command.SourceID != "camera-main" || command.SourceClass != SourceUSBCapture || string(command.Parameters) != string(parameters) {
		t.Fatalf("unexpected forwarded LiveSource command: %+v", command)
	}
}

func TestForwarderRejectsNonLiveCapabilityBeforeBackend(t *testing.T) {
	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	s := store.New(handle.DB, clock.Real{})
	called := false
	forwarder := NewForwarder(s, capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		called = true
		return capability.Result{}
	}))
	result := forwarder.Execute(ctx, capability.Request{
		Capability: "osc.send",
		Target: &capability.Target{LogicalType: LogicalType, Configuration: json.RawMessage(`{}`)},
	})
	if called {
		t.Fatal("unsupported capability reached Companion backend")
	}
	if result.ErrorCode != "LIVE_SOURCE_CAPABILITY_UNSUPPORTED" {
		t.Fatalf("unexpected error code: %s", result.ErrorCode)
	}
}
