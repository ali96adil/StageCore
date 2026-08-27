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
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestCueTargetsMachineRoleAndReplacementNeedsNoCueEdit(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	s := store.New(h.DB, clock.Real{})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "M4 Companion Show", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	role, err := s.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey:              "VIDEO-MAIN",
		DisplayName:          "Video Main",
		RequiredCapabilities: []string{"sim.test"},
		Required:             true,
	})
	if err != nil {
		t.Fatal(err)
	}
	roleConfig, _ := json.Marshal(map[string]any{"machine_role_id": role.ID})
	if _, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID:     project.ID,
		LogicalName:   "VIDEO-MAIN",
		LogicalType:   companion.MachineRoleLogicalType,
		TargetRef:     "VIDEO-MAIN",
		ProjectConfig: roleConfig,
	}); err != nil {
		t.Fatal(err)
	}
	cue, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID:   revision.ID,
		DisplayLabel: "1",
		Name:         "Video Main Cue",
		OrderIndex:   0,
		Enabled:      true,
	}, []domain.Action{{
		OrderIndex:    0,
		ExecutionMode: "SEQUENTIAL",
		TargetRef:     "VIDEO-MAIN",
		CapabilityKey: "sim.test",
		Parameters:    json.RawMessage(`{"message":"go"}`),
		ErrorPolicy:   json.RawMessage(`{"on_error":"FAIL_CUE"}`),
		PriorityClass: domain.PriorityP1,
		Enabled:       true,
	}})
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

	channel := companionchannel.NewSimulated()
	registry := capability.NewRegistry()
	forwarder := companion.NewForwarder(s, channel, time.Minute, time.Now)
	if err := registry.RegisterTargetType(companion.MachineRoleLogicalType, forwarder); err != nil {
		t.Fatal(err)
	}
	engine := cueengine.NewWithExecutor(s, registry)

	first := prepareCompanion(t, ctx, s, channel, runtimeSnapshot.ID, "Video Mac A", "video-a.local")
	assignment, err := s.AssignMachineRole(ctx, role.ID, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	firstSession, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "first machine")
	if err != nil {
		t.Fatal(err)
	}
	firstCommand := companionCueCommand(t, project.ID, runtimeSnapshot.ID)
	firstResult := engine.ExecuteCueGo(ctx, firstSession.ID, firstCommand)
	if firstResult.Status != contracts.CommandCompleted {
		t.Fatalf("first result=%#v", firstResult)
	}
	if got := channel.ExecutionCount(first.ID); got != 1 {
		t.Fatalf("first Companion execution count=%d want 1", got)
	}

	// Transport reconnect must not replay the previously completed non-idempotent
	// execution. The Hub returns the stored command result before any new dispatch.
	if err := channel.SetConnected(first.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := channel.SetConnected(first.ID, true); err != nil {
		t.Fatal(err)
	}
	duplicateResult := engine.ExecuteCueGo(ctx, firstSession.ID, firstCommand)
	if duplicateResult.Status != contracts.CommandCompleted {
		t.Fatalf("duplicate result=%#v", duplicateResult)
	}
	if got := channel.ExecutionCount(first.ID); got != 1 {
		t.Fatalf("reconnect replayed previous execution: count=%d", got)
	}

	if err := s.ReleaseRoleAssignment(ctx, assignment.ID); err != nil {
		t.Fatal(err)
	}
	second := prepareCompanion(t, ctx, s, channel, runtimeSnapshot.ID, "Video Mac B", "video-b.local")
	if _, err := s.AssignMachineRole(ctx, role.ID, second.ID); err != nil {
		t.Fatal(err)
	}
	secondSession, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "replacement machine")
	if err != nil {
		t.Fatal(err)
	}
	secondResult := engine.ExecuteCueGo(ctx, secondSession.ID, companionCueCommand(t, project.ID, runtimeSnapshot.ID))
	if secondResult.Status != contracts.CommandCompleted {
		t.Fatalf("replacement result=%#v", secondResult)
	}
	if got := channel.ExecutionCount(first.ID); got != 1 {
		t.Fatalf("old Companion received replacement execution: count=%d", got)
	}
	if got := channel.ExecutionCount(second.ID); got != 1 {
		t.Fatalf("replacement Companion execution count=%d want 1", got)
	}

	// Both runs used the same immutable Snapshot and therefore the same Cue
	// definition. Machine replacement changed assignment only, never Cue data.
	if firstSession.RuntimeSnapshotID != secondSession.RuntimeSnapshotID || firstSession.RuntimeSnapshotID != runtimeSnapshot.ID {
		t.Fatalf("replacement changed Runtime Snapshot: first=%s second=%s", firstSession.RuntimeSnapshotID, secondSession.RuntimeSnapshotID)
	}
	if cue.ID == "" {
		t.Fatal("fixture Cue was not created")
	}
}

func prepareCompanion(
	t *testing.T,
	ctx context.Context,
	s *store.Store,
	channel *companionchannel.SimulatedChannel,
	runtimeSnapshotID, displayName, hostname string,
) domain.Companion {
	t.Helper()
	c, err := s.RegisterCompanion(ctx, store.RegisterCompanionParams{
		DisplayName:  displayName,
		Hostname:     hostname,
		Platform:     "darwin",
		Architecture: "arm64",
		Version:      "0.1.0",
		Capabilities: []string{"sim.test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCompanionTrustState(ctx, c.ID, domain.CompanionTrusted); err != nil {
		t.Fatal(err)
	}
	c, err = s.UpdateCompanionReport(ctx, c.ID, store.CompanionReportParams{
		DisplayName:              displayName,
		Hostname:                 hostname,
		Platform:                 "darwin",
		Architecture:             "arm64",
		Version:                  "0.1.0",
		Capabilities:             []string{"sim.test"},
		Readiness:                domain.CompanionReadinessReady,
		AppliedRuntimeSnapshotID: &runtimeSnapshotID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := channel.RegisterAgent(companionchannel.AgentConfig{
		CompanionID:       c.ID,
		AppliedSnapshotID: runtimeSnapshotID,
		Capabilities:      []string{"sim.test"},
		Connected:         true,
	}); err != nil {
		t.Fatal(err)
	}
	return c
}

func companionCueCommand(t *testing.T, projectID, runtimeSnapshotID string) contracts.CommandEnvelope {
	t.Helper()
	commandID, err := stageid.New()
	if err != nil {
		t.Fatal(err)
	}
	return contracts.CommandEnvelope{
		CommandID:         commandID,
		CommandType:       cueengine.CueGoCommandType,
		SchemaVersion:     contracts.SchemaVersion1,
		IssuedAt:          time.Now().UTC(),
		ProjectID:         projectID,
		RuntimeSnapshotID: runtimeSnapshotID,
		Issuer:            "test.operator",
		Priority:          "P1",
		Payload:           json.RawMessage(`{}`),
	}
}
