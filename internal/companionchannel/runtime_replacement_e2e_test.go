package companionchannel_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companion"
	"github.com/ali96adil/StageCore/internal/companionauth"
	"github.com/ali96adil/StageCore/internal/companionchannel"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/httpapi"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestAuthenticatedRuntimeReplacementUsesSameCueAndSnapshot(t *testing.T) {
	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()

	s := store.New(handle.DB, clock.Real{})
	auth := companionauth.New(s, nil)
	runtime := companionchannel.NewRuntime(s, auth)
	defer runtime.Close()

	companionA := "11111111-1111-4111-8111-111111111111"
	privateA, publicA := runtimeDeviceKey(t)
	pairRuntimeCompanion(t, ctx, auth, companionA, publicA)
	project, _, role, cue, runtimeSnapshot := runtimeCueFixture(t, ctx, s, companionA)

	server := httptest.NewServer(httpapi.New(
		httpapi.WithCompanionAuth(auth),
		httpapi.WithCompanionRuntime(runtime),
	).Handler())
	defer server.Close()
	runtimeURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/companion/runtime"

	memoryA := &runtimeAgentMemory{seen: make(map[string]struct{})}
	credentialA := authenticateRuntimeCompanion(t, ctx, auth, companionA, privateA)
	agentA := startRuntimeAgent(t, runtimeURL, credentialA.Token, companionA, memoryA)
	waitForRuntime(t, func() bool {
		return runtime.IsConnected(companionA) && companionReady(ctx, s, companionA, runtimeSnapshot.ID)
	})

	registry := capability.NewRegistry()
	if err := registry.RegisterTargetType(
		companion.MachineRoleLogicalType,
		companion.NewForwarder(s, runtime, 5*time.Second, nil),
	); err != nil {
		t.Fatal(err)
	}
	engine := cueengine.NewWithExecutor(s, registry)

	firstSession, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "replacement source")
	if err != nil {
		t.Fatal(err)
	}
	firstResult := engine.ExecuteCueGo(ctx, firstSession.ID, runtimeCueCommand(t, project.ID, runtimeSnapshot.ID))
	if firstResult.Status != contracts.CommandCompleted {
		t.Fatalf("first Companion result=%#v", firstResult)
	}
	if got := memoryA.count.Load(); got != 1 {
		t.Fatalf("first Companion execution count=%d want 1", got)
	}
	firstExecutions, err := s.ListCueExecutions(ctx, firstSession.ID)
	if err != nil || len(firstExecutions) != 1 || firstExecutions[0].CueID != cue.ID {
		t.Fatalf("first Cue execution=%#v err=%v want cue=%s", firstExecutions, err, cue.ID)
	}

	assignmentA, err := s.GetActiveRoleAssignment(ctx, role.ID)
	if err != nil {
		t.Fatal(err)
	}
	agentA.close(t)
	waitForRuntime(t, func() bool { return !runtime.IsConnected(companionA) })
	if err := s.ReleaseRoleAssignment(ctx, assignmentA.ID); err != nil {
		t.Fatal(err)
	}

	companionB := "22222222-2222-4222-8222-222222222222"
	privateB, publicB := runtimeDeviceKey(t)
	pairRuntimeCompanion(t, ctx, auth, companionB, publicB)
	assignmentB, err := s.AssignMachineRole(ctx, role.ID, companionB)
	if err != nil {
		t.Fatal(err)
	}
	if assignmentB.CompanionID != companionB {
		t.Fatalf("replacement assignment Companion=%s want %s", assignmentB.CompanionID, companionB)
	}

	memoryB := &runtimeAgentMemory{seen: make(map[string]struct{})}
	credentialB := authenticateRuntimeCompanion(t, ctx, auth, companionB, privateB)
	agentB := startRuntimeAgent(t, runtimeURL, credentialB.Token, companionB, memoryB)
	defer agentB.close(t)
	waitForRuntime(t, func() bool {
		return runtime.IsConnected(companionB) && companionReady(ctx, s, companionB, runtimeSnapshot.ID)
	})

	if got := memoryA.count.Load(); got != 1 {
		t.Fatalf("old Companion received replacement work before new Cue: count=%d", got)
	}
	if got := memoryB.count.Load(); got != 0 {
		t.Fatalf("replacement connection replayed old work: count=%d", got)
	}

	secondSession, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "replacement target")
	if err != nil {
		t.Fatal(err)
	}
	secondResult := engine.ExecuteCueGo(ctx, secondSession.ID, runtimeCueCommand(t, project.ID, runtimeSnapshot.ID))
	if secondResult.Status != contracts.CommandCompleted {
		t.Fatalf("replacement Companion result=%#v", secondResult)
	}
	if got := memoryA.count.Load(); got != 1 {
		t.Fatalf("old Companion received replacement execution: count=%d", got)
	}
	if got := memoryB.count.Load(); got != 1 {
		t.Fatalf("replacement Companion execution count=%d want 1", got)
	}

	secondExecutions, err := s.ListCueExecutions(ctx, secondSession.ID)
	if err != nil || len(secondExecutions) != 1 || secondExecutions[0].CueID != cue.ID {
		t.Fatalf("replacement Cue execution=%#v err=%v want cue=%s", secondExecutions, err, cue.ID)
	}
	if firstSession.RuntimeSnapshotID != secondSession.RuntimeSnapshotID || secondSession.RuntimeSnapshotID != runtimeSnapshot.ID {
		t.Fatalf(
			"replacement changed Runtime Snapshot: first=%s second=%s expected=%s",
			firstSession.RuntimeSnapshotID,
			secondSession.RuntimeSnapshotID,
			runtimeSnapshot.ID,
		)
	}
}
