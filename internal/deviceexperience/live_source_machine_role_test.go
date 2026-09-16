package deviceexperience_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestLiveSourceMachineRoleBindingPersistsAndRejectsAmbiguousPlacement(t *testing.T) {
	ctx := context.Background()
	repo, h, projectID := newRepository(t)
	stageStore := store.New(h.DB, clock.Fixed{Time: phase4Time})
	role, err := stageStore.CreateMachineRole(ctx, projectID, store.CreateMachineRoleParams{
		RoleKey: "visual-render", DisplayName: "Visual Render Node",
		RequiredCapabilities: deviceexperience.LiveSourceCapabilityKeys(), Required: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	created, err := repo.UpsertLiveSource(ctx, deviceexperience.LiveSource{
		ProjectID: projectID,
		Name: "Main Camera",
		Class: deviceexperience.SourceLocalCamera,
		ExecutionMachineRoleID: role.ID,
		Capabilities: []string{deviceexperience.CapabilityVideoSourceOpen, deviceexperience.CapabilityVideoSourceRoute},
		Config: json.RawMessage(`{"device_ref":"camera-main"}`),
		Required: true,
		DesiredEnabled: true,
		Readiness: deviceexperience.ReadinessUnknown,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ExecutionMachineRoleID != role.ID || created.ExecutionDeviceID != "" {
		t.Fatalf("unexpected execution placement: %+v", created)
	}
	loaded, err := repo.GetLiveSource(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ExecutionMachineRoleID != role.ID {
		t.Fatalf("machine role binding=%q want %q", loaded.ExecutionMachineRoleID, role.ID)
	}

	_, err = repo.UpsertDevice(ctx, deviceexperience.Device{
		ID: "render-device", ProjectID: projectID, Kind: deviceexperience.DeviceRenderNode,
		DisplayName: "Legacy Render Device", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: deviceexperience.LiveSourceCapabilityKeys(), Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	created.ExecutionDeviceID = "render-device"
	if _, err := repo.UpsertLiveSource(ctx, created); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("ambiguous execution placement err=%v", err)
	}
}

func TestLiveSourceMachineRoleMustBelongToSameProject(t *testing.T) {
	ctx := context.Background()
	repo, h, projectID := newRepository(t)
	stageStore := store.New(h.DB, clock.Fixed{Time: phase4Time})
	otherProject, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Other", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	otherRole, err := stageStore.CreateMachineRole(ctx, otherProject.ID, store.CreateMachineRoleParams{
		RoleKey: "other-render", DisplayName: "Other Render Node",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = repo.UpsertLiveSource(ctx, deviceexperience.LiveSource{
		ProjectID: projectID, Name: "Wrong Role Camera", Class: deviceexperience.SourceUSBCapture,
		ExecutionMachineRoleID: otherRole.ID, DesiredEnabled: true,
	})
	if !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("cross-project Machine Role err=%v", err)
	}
}
