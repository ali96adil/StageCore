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
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestCuePersistsCompanionTimeoutWithoutFalseSuccess(t *testing.T) {
	result, execution := runCompanionCueWithBehavior(t, companionchannel.SimulationTimeout)
	if result.Status != contracts.CommandTimedOut {
		t.Fatalf("command result=%#v", result)
	}
	if execution.Result != domain.ExecutionTimedOut || execution.ErrorCode == nil || *execution.ErrorCode != "COMPANION_EXECUTION_TIMEOUT" {
		t.Fatalf("action execution=%#v", execution)
	}
}

func TestCuePersistsInterruptedCompanionExecutionWithoutFalseSuccess(t *testing.T) {
	result, execution := runCompanionCueWithBehavior(t, companionchannel.SimulationInterrupted)
	if result.Status != contracts.CommandFailed {
		t.Fatalf("command result=%#v", result)
	}
	if execution.Result != domain.ExecutionFailed || execution.ErrorCode == nil || *execution.ErrorCode != "COMPANION_EXECUTION_INTERRUPTED" {
		t.Fatalf("action execution=%#v", execution)
	}
}

func runCompanionCueWithBehavior(t *testing.T, behavior companionchannel.SimulationBehavior) (contracts.CommandResult, domain.ActionExecution) {
	t.Helper()
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	s := store.New(h.DB, clock.Real{})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "M4 terminal result show", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	role, err := s.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey:              "VIDEO-MAIN",
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
	if _, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID:   revision.ID,
		DisplayLabel: "1",
		Name:         "Companion terminal result cue",
		OrderIndex:   0,
		Enabled:      true,
	}, []domain.Action{{
		OrderIndex:    0,
		ExecutionMode: "SEQUENTIAL",
		TargetRef:     "VIDEO-MAIN",
		CapabilityKey: "sim.test",
		Parameters:    json.RawMessage(`{"message":"go"}`),
		TimeoutPolicy: json.RawMessage(`{"timeout_ms":500}`),
		ErrorPolicy:   json.RawMessage(`{"on_error":"FAIL_CUE"}`),
		PriorityClass: domain.PriorityP1,
		Enabled:       true,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}

	c, err := s.RegisterCompanion(ctx, store.RegisterCompanionParams{
		DisplayName:  "Terminal Result Mac",
		Hostname:     "terminal.local",
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
		DisplayName:              c.DisplayName,
		Hostname:                 c.Hostname,
		Platform:                 c.Platform,
		Architecture:             c.Architecture,
		Version:                  c.Version,
		Capabilities:             []string{"sim.test"},
		Readiness:                domain.CompanionReadinessReady,
		AppliedRuntimeSnapshotID: &runtimeSnapshot.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignMachineRole(ctx, role.ID, c.ID); err != nil {
		t.Fatal(err)
	}

	channel := companionchannel.NewSimulated()
	if err := channel.RegisterAgent(companionchannel.AgentConfig{
		CompanionID:       c.ID,
		AppliedSnapshotID: runtimeSnapshot.ID,
		Capabilities:      []string{"sim.test"},
		Connected:         true,
		Behavior:          behavior,
	}); err != nil {
		t.Fatal(err)
	}
	registry := capability.NewRegistry()
	if err := registry.RegisterTargetType(
		companion.MachineRoleLogicalType,
		companion.NewForwarder(s, channel, time.Minute, time.Now),
	); err != nil {
		t.Fatal(err)
	}
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "terminal result")
	if err != nil {
		t.Fatal(err)
	}
	result := cueengine.NewWithExecutor(s, registry).ExecuteCueGo(ctx, session.ID, companionCueCommand(t, project.ID, runtimeSnapshot.ID))

	cueExecutions, err := s.ListCueExecutions(ctx, session.ID)
	if err != nil || len(cueExecutions) != 1 {
		t.Fatalf("cue executions=%#v err=%v", cueExecutions, err)
	}
	actionExecutions, err := s.ListActionExecutions(ctx, cueExecutions[0].ID)
	if err != nil || len(actionExecutions) != 1 {
		t.Fatalf("action executions=%#v err=%v", actionExecutions, err)
	}
	return result, actionExecutions[0]
}
