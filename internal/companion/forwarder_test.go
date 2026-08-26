package companion_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companion"
	"github.com/ali96adil/StageCore/internal/companionchannel"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestForwarderExecutesReadyMachineRoleAndPreservesExecutionIdentity(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 26, 17, 40, 0, 0, time.UTC)
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Fixed{Time: now})

	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Forwarder Show"})
	if err != nil {
		t.Fatal(err)
	}
	role, err := s.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey:              "VIDEO-MAIN",
		RequiredCapabilities: []string{"osc.send"},
		Required:             true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}

	companionState, err := s.RegisterCompanion(ctx, store.RegisterCompanionParams{
		DisplayName:  "Video Mac",
		Platform:     "darwin",
		Architecture: "arm64",
		Capabilities: []string{"osc.send"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCompanionTrustState(ctx, companionState.ID, domain.CompanionTrusted); err != nil {
		t.Fatal(err)
	}
	companionState, err = s.UpdateCompanionReport(ctx, companionState.ID, store.CompanionReportParams{
		DisplayName:              "Video Mac",
		Platform:                 "darwin",
		Architecture:             "arm64",
		Capabilities:             []string{"osc.send"},
		Readiness:                domain.CompanionReadinessReady,
		AppliedRuntimeSnapshotID: &runtimeSnapshot.ID,
		ConfigHash:               "",
	})
	if err != nil {
		t.Fatal(err)
	}
	assignment, err := s.AssignMachineRole(ctx, role.ID, companionState.ID)
	if err != nil {
		t.Fatal(err)
	}

	channel := companionchannel.NewSimulated()
	if err := channel.RegisterAgent(companionchannel.AgentConfig{
		CompanionID:       companionState.ID,
		AppliedSnapshotID: runtimeSnapshot.ID,
		Capabilities:      []string{"osc.send"},
		Connected:         true,
	}); err != nil {
		t.Fatal(err)
	}
	forwarder := companion.NewForwarder(s, channel, 5*time.Second, func() time.Time { return now })
	targetConfig, _ := json.Marshal(map[string]string{"machine_role_id": role.ID})
	request := capability.Request{
		ExecutionID:       "execution-1",
		RuntimeSnapshotID: runtimeSnapshot.ID,
		Capability:        "osc.send",
		Target: &capability.Target{
			Ref:           "VIDEO-MAIN",
			LogicalType:   companion.MachineRoleLogicalType,
			Configuration: targetConfig,
		},
		CorrelationID: "correlation-1",
	}

	first := forwarder.Execute(ctx, request)
	if first.Result != domain.ExecutionCompleted || first.AckLevel != contracts.AckAccepted {
		t.Fatalf("first=%#v", first)
	}
	if got := channel.ExecutionCount(companionState.ID); got != 1 {
		t.Fatalf("execution count=%d want 1", got)
	}
	assignment, err = s.GetRoleAssignment(ctx, assignment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if assignment.State != domain.RoleReady {
		t.Fatalf("assignment state=%s want READY", assignment.State)
	}

	if err := channel.SetConnected(companionState.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := channel.SetConnected(companionState.ID, true); err != nil {
		t.Fatal(err)
	}
	duplicate := forwarder.Execute(ctx, request)
	if duplicate.Result != first.Result || duplicate.AckLevel != first.AckLevel || duplicate.ErrorCode != first.ErrorCode {
		t.Fatalf("duplicate=%#v first=%#v", duplicate, first)
	}
	if got := channel.ExecutionCount(companionState.ID); got != 1 {
		t.Fatalf("duplicate replayed execution; count=%d want 1", got)
	}
}

func TestForwarderRejectsCompanionAppliedSnapshotMismatchBeforeDispatch(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 26, 17, 41, 0, 0, time.UTC)
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Fixed{Time: now})

	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Snapshot Gate"})
	if err != nil {
		t.Fatal(err)
	}
	role, err := s.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey:              "VIDEO-MAIN",
		RequiredCapabilities: []string{"osc.send"},
		Required:             true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	oldSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "old")
	if err != nil {
		t.Fatal(err)
	}
	newSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "new")
	if err != nil {
		t.Fatal(err)
	}

	companionState, err := s.RegisterCompanion(ctx, store.RegisterCompanionParams{DisplayName: "Video Mac", Capabilities: []string{"osc.send"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCompanionTrustState(ctx, companionState.ID, domain.CompanionTrusted); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateCompanionReport(ctx, companionState.ID, store.CompanionReportParams{
		DisplayName:              "Video Mac",
		Capabilities:             []string{"osc.send"},
		Readiness:                domain.CompanionReadinessReady,
		AppliedRuntimeSnapshotID: &oldSnapshot.ID,
	}); err != nil {
		t.Fatal(err)
	}
	assignment, err := s.AssignMachineRole(ctx, role.ID, companionState.ID)
	if err != nil {
		t.Fatal(err)
	}

	channel := companionchannel.NewSimulated()
	if err := channel.RegisterAgent(companionchannel.AgentConfig{
		CompanionID:       companionState.ID,
		AppliedSnapshotID: oldSnapshot.ID,
		Capabilities:      []string{"osc.send"},
		Connected:         true,
	}); err != nil {
		t.Fatal(err)
	}
	forwarder := companion.NewForwarder(s, channel, 5*time.Second, func() time.Time { return now })
	targetConfig, _ := json.Marshal(map[string]string{"machine_role_id": role.ID})
	result := forwarder.Execute(ctx, capability.Request{
		ExecutionID:       "execution-stale",
		RuntimeSnapshotID: newSnapshot.ID,
		Capability:        "osc.send",
		Target: &capability.Target{
			Ref:           "VIDEO-MAIN",
			LogicalType:   companion.MachineRoleLogicalType,
			Configuration: targetConfig,
		},
	})
	if result.Result != domain.ExecutionFailed || result.ErrorCode != "SNAPSHOT_MISMATCH" || result.AckLevel != contracts.AckNone {
		t.Fatalf("result=%#v", result)
	}
	if got := channel.ExecutionCount(companionState.ID); got != 0 {
		t.Fatalf("mismatched Snapshot dispatched; count=%d", got)
	}
	assignment, err = s.GetRoleAssignment(ctx, assignment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if assignment.State != domain.RoleMismatch {
		t.Fatalf("assignment state=%s want MISMATCH", assignment.State)
	}
}
